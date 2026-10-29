package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/rikimela1992/vodmax/internal/store"
)

// Recording rules record guide programmes automatically ("every Juventus
// match", "F1, race and qualifying"). They run after every guide refresh
// and every rulesEvery, over the programmes of the next guideFuture.

const (
	rulesEvery   = 10 * time.Minute
	maxRuleWords = 20
)

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// fold lowers case and removes accents and punctuation.
func fold(s string) string {
	s = strings.ToLower(norm.NFKD.String(s))
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(s, " ")), " ")
}

// hasWord: a single word matches a word of text that starts with it; a
// phrase matches as written, on word boundaries.
func hasWord(text, word string) bool {
	w := fold(word)
	if w == "" {
		return false
	}
	if strings.Contains(w, " ") {
		return strings.Contains(" "+text+" ", " "+w+" ")
	}
	for _, t := range strings.Fields(text) {
		if strings.HasPrefix(t, w) {
			return true
		}
	}
	return false
}

func ruleMatches(spec store.RuleSpec, p store.Programme) bool {
	text := fold(p.Title + " " + p.Subtitle)
	for _, w := range spec.All {
		if !hasWord(text, w) {
			return false
		}
	}
	if len(spec.Any) > 0 {
		ok := false
		for _, w := range spec.Any {
			if hasWord(text, w) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, w := range spec.None {
		if hasWord(text, w) {
			return false
		}
	}
	return true
}

func cleanWords(list []string) []string {
	out := []string{}
	for _, w := range list {
		if w = strings.TrimSpace(w); w != "" && fold(w) != "" {
			out = append(out, w)
		}
	}
	return out
}

func (s *Service) checkRule(r *store.RecordingRule) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Spec.All, r.Spec.Any, r.Spec.None = cleanWords(r.Spec.All), cleanWords(r.Spec.Any), cleanWords(r.Spec.None)
	switch {
	case r.Name == "" || len(r.Name) > 60:
		return invalid("nome della regola obbligatorio (max 60 caratteri)")
	case len(r.Spec.All) == 0:
		return invalid("serve almeno una parola che il programma deve contenere")
	case len(r.Spec.All)+len(r.Spec.Any)+len(r.Spec.None) > maxRuleWords:
		return invalid("troppe parole (max %d)", maxRuleWords)
	case r.Keep < 0 || r.Keep > 1000:
		return invalid("numero di registrazioni da tenere non valido")
	}
	switch r.Spec.Section {
	case "", "tv", "sport", "radio":
	default:
		return invalid("sezione non valida")
	}
	return nil
}

// RuleMatch is a programme a rule would record.
type RuleMatch struct {
	ChannelID int64  `json:"channel_id"`
	Channel   string `json:"channel"`
	Start     int64  `json:"start"`
	Stop      int64  `json:"stop"`
	Title     string `json:"title"`
	State     string `json:"state"` // new, scheduled, cancelled, conflict, too_long
	Note      string `json:"note,omitempty"`

	group string
	sub   int64
}

// ruleCandidates finds the programmes of the next guideFuture a rule
// matches: one channel per event (the same match on several channels is
// recorded once), ending at least five minutes from now.
func (s *Service) ruleCandidates(ctx context.Context, spec store.RuleSpec) ([]RuleMatch, error) {
	now := time.Now()
	channels, err := s.st.TVChannels(ctx, DefaultProfile, false, spec.Section, false, now)
	if err != nil {
		return nil, err
	}
	var guides []string
	for _, c := range channels {
		if c.GuideID != "" && (spec.Channel == "" || c.GroupKey == spec.Channel) {
			guides = append(guides, c.GuideID)
		}
	}
	progs, err := s.st.GuideWindow(ctx, guides, now.Add(5*time.Minute), now.Add(guideFuture))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []RuleMatch
	for _, c := range channels { // remote-control order: official channels first
		if c.GuideID == "" || (spec.Channel != "" && c.GroupKey != spec.Channel) {
			continue
		}
		it, err := s.st.PlaybackItem(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, p := range progs[c.GuideID] {
			if !ruleMatches(spec, p) {
				continue
			}
			event := fold(p.Title) + "|" + fmt.Sprint(p.Start/600)
			if seen[event] {
				continue
			}
			seen[event] = true
			out = append(out, RuleMatch{ChannelID: c.ID, Channel: c.Name, Start: p.Start, Stop: p.Stop, Title: p.Title,
				group: c.GroupKey, sub: it.SubscriptionID})
		}
	}
	return out, nil
}

// assess says what would happen to a candidate.
func (s *Service) assess(ctx context.Context, m *RuleMatch) error {
	start, stop := time.Unix(m.Start, 0).Add(-recordBefore).Unix(), time.Unix(m.Stop, 0).Add(recordAfter).Unix()
	switch status, err := s.st.RecordingCovers(ctx, m.group, m.Start+60, m.Stop-60); {
	case err != nil:
		return err
	case status == "cancelled":
		m.State, m.Note = "cancelled", "annullata a mano"
		return nil
	case status != "":
		m.State = "scheduled"
		return nil
	}
	if time.Duration(stop-start)*time.Second > maxRecording {
		m.State, m.Note = "too_long", "più lungo di 6 ore"
		return nil
	}
	why, err := s.recordingConflict(ctx, m.sub, m.group, start, stop)
	if err != nil {
		return err
	}
	if why != "" {
		m.State, m.Note = "conflict", why
		return nil
	}
	m.State = "new"
	return nil
}

// PreviewRule lists what a rule would record now.
func (s *Service) PreviewRule(ctx context.Context, r store.RecordingRule) ([]RuleMatch, error) {
	if err := s.checkRule(&r); err != nil {
		return nil, err
	}
	list, err := s.ruleCandidates(ctx, r.Spec)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := s.assess(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	if list == nil {
		list = []RuleMatch{}
	}
	return list, nil
}

// applyRules schedules what the enabled rules match.
func (s *Service) applyRules(ctx context.Context, log *slog.Logger) {
	rules, err := s.st.RecordingRules(ctx)
	if err != nil {
		return
	}
	for _, r := range rules {
		if !r.Enabled || ctx.Err() != nil {
			continue
		}
		note, err := s.applyRule(ctx, r)
		if err != nil {
			note = "errore: " + err.Error()
			log.Warn("regola di registrazione", "regola", r.Name, "err", err)
		}
		s.st.SetRecordingRuleResult(ctx, r.ID, note)
	}
	s.mu.Lock()
	s.maint.rulesRun = time.Now()
	s.mu.Unlock()
}

func (s *Service) applyRule(ctx context.Context, r store.RecordingRule) (string, error) {
	list, err := s.ruleCandidates(ctx, r.Spec)
	if err != nil {
		return "", err
	}
	added, already, cancelled := 0, 0, 0
	var conflicts []string
	for i := range list {
		m := &list[i]
		if err := s.assess(ctx, m); err != nil {
			return "", err
		}
		switch m.State {
		case "scheduled":
			already++
		case "cancelled":
			cancelled++
		case "conflict":
			conflicts = append(conflicts, fmt.Sprintf("“%s” %s", m.Title, time.Unix(m.Start, 0).Format("02/01 15:04")))
		case "new":
			it, err := s.st.PlaybackItem(ctx, m.ChannelID)
			if err != nil {
				continue
			}
			rec := store.Recording{ItemID: it.ID, GroupKey: m.group, Channel: m.Channel, Title: m.Title, RuleID: r.ID,
				Start: time.Unix(m.Start, 0).Add(-recordBefore).Unix(), Stop: time.Unix(m.Stop, 0).Add(recordAfter).Unix()}
			if _, err := s.st.AddRecording(ctx, rec); err != nil {
				return "", err
			}
			added++
		}
	}
	note := fmt.Sprintf("%d in programma", added+already)
	if added > 0 {
		note += fmt.Sprintf(" (%d nuove)", added)
	}
	if cancelled > 0 {
		note += fmt.Sprintf(", %d annullate a mano", cancelled)
	}
	if len(list) == 0 {
		note = "nessun programma corrispondente nella guida dei prossimi giorni"
	}
	if len(conflicts) > 0 {
		note += "; non registrabili per mancanza di connessioni: " + strings.Join(conflicts, ", ")
	}
	return note, nil
}

// ---- Rule management ----

func (s *Service) RecordingRules(ctx context.Context) ([]store.RecordingRule, error) {
	list, err := s.st.RecordingRules(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if g := list[i].Spec.Channel; g != "" {
			if it, err := s.st.ChannelForGroup(ctx, g); err == nil {
				list[i].ChannelName = it.Title
			}
		}
	}
	return list, nil
}

// RuleInput creates or changes a rule; Channel is a channel id (0: any).
type RuleInput struct {
	Name      string   `json:"name"`
	All       []string `json:"all"`
	Any       []string `json:"any"`
	None      []string `json:"none"`
	ChannelID int64    `json:"channel_id"`
	Section   string   `json:"section"`
	Keep      int      `json:"keep"`
	Enabled   *bool    `json:"enabled"`
}

func (s *Service) ruleFromInput(ctx context.Context, in RuleInput) (store.RecordingRule, error) {
	r := store.RecordingRule{Name: in.Name, Keep: in.Keep, Enabled: in.Enabled == nil || *in.Enabled,
		Spec: store.RuleSpec{All: in.All, Any: in.Any, None: in.None, Section: in.Section}}
	if in.ChannelID != 0 {
		g, err := s.st.GroupKeyOf(ctx, in.ChannelID)
		if err != nil {
			return r, invalid("canale inesistente")
		}
		r.Spec.Channel = g
	}
	return r, s.checkRule(&r)
}

func (s *Service) PreviewRuleInput(ctx context.Context, in RuleInput) ([]RuleMatch, error) {
	r, err := s.ruleFromInput(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.PreviewRule(ctx, r)
}

func (s *Service) AddRecordingRule(ctx context.Context, in RuleInput) (store.RecordingRule, error) {
	r, err := s.ruleFromInput(ctx, in)
	if err != nil {
		return r, err
	}
	if r.ID, err = s.st.AddRecordingRule(ctx, r); err != nil {
		return r, err
	}
	s.runRule(ctx, r.ID)
	return s.st.RecordingRule(ctx, r.ID)
}

func (s *Service) UpdateRecordingRule(ctx context.Context, id int64, in RuleInput) (store.RecordingRule, error) {
	r, err := s.ruleFromInput(ctx, in)
	if err != nil {
		return r, err
	}
	r.ID = id
	if err := s.st.UpdateRecordingRule(ctx, r); err != nil {
		return r, err
	}
	s.runRule(ctx, id)
	return s.st.RecordingRule(ctx, id)
}

func (s *Service) DeleteRecordingRule(ctx context.Context, id int64) error {
	return s.st.DeleteRecordingRule(ctx, id)
}

// runRule applies one rule now, so the user sees its effect at once.
func (s *Service) runRule(ctx context.Context, id int64) {
	r, err := s.st.RecordingRule(ctx, id)
	if err != nil || !r.Enabled {
		return
	}
	note, err := s.applyRule(ctx, r)
	if err != nil {
		note = "errore: " + err.Error()
	}
	s.st.SetRecordingRuleResult(ctx, id, note)
	s.scheduleDue(context.WithoutCancel(ctx))
}
