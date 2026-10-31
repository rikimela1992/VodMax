package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silirim/vodmax/internal/store"
)

// The assistant: a language model chosen by the admin (Ollama, LM Studio,
// llama.cpp at home, or a service on the internet), reached through the
// OpenAI-compatible chat API. It turns a sentence into a recording rule
// and picks something to watch tonight. Off by default. The model only
// proposes: a rule is filled in for the person to check, and tonight's
// picks can only be programmes and titles VodMax gave it, by code.

const (
	assistantKey       = "assistant"
	assistantSecretKey = "assistant_key"
	assistantTimeout   = 5 * time.Minute // a small model on a processor reads ~50 tokens a second
	tonightCache       = 20 * time.Minute
)

var (
	ErrAssistantOff = errors.New("l'assistente non è configurato: Impostazioni → Sistema → Assistente")
	ErrAssistant    = errors.New("l'assistente ha dato una risposta che VodMax non sa usare: riprova o cambia modello")
)

// AssistantSettings: URL is the base of the API (for example
// http://ollama:11434/v1); the key, when needed, is kept apart, sealed.
type AssistantSettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Model   string `json:"model"`
}

type assistantState struct {
	mu      sync.Mutex
	tonight map[string]tonightEntry // profile + wish
	client  *http.Client            // tests
}

type tonightEntry struct {
	at  time.Time
	out Tonight
}

func (s *Service) AssistantSettings(ctx context.Context) (AssistantSettings, bool, error) {
	var a AssistantSettings
	if err := s.st.Setting(ctx, assistantKey, &a); err != nil {
		return a, false, err
	}
	key, err := s.secretSetting(ctx, assistantSecretKey)
	return a, key != "", err
}

// AssistantInput changes the settings; Key nil keeps the current one, ""
// removes it.
type AssistantInput struct {
	Enabled bool    `json:"enabled"`
	URL     string  `json:"url"`
	Model   string  `json:"model"`
	Key     *string `json:"key"`
}

func (s *Service) SetAssistant(ctx context.Context, in AssistantInput) error {
	a := AssistantSettings{Enabled: in.Enabled, URL: strings.TrimRight(strings.TrimSpace(in.URL), "/"), Model: strings.TrimSpace(in.Model)}
	if a.URL != "" {
		u, err := url.Parse(a.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return invalid("indirizzo dell'assistente non valido (atteso http:// o https://)")
		}
		a.URL = strings.TrimSuffix(a.URL, "/chat/completions")
	}
	if a.Enabled && (a.URL == "" || a.Model == "") {
		return invalid("per accendere l'assistente servono indirizzo e modello")
	}
	if len(a.Model) > 100 {
		return invalid("nome del modello troppo lungo")
	}
	if in.Key != nil {
		if err := s.setSecretSetting(ctx, assistantSecretKey, strings.TrimSpace(*in.Key)); err != nil {
			return err
		}
	}
	s.asst.mu.Lock()
	s.asst.tonight = nil
	s.asst.mu.Unlock()
	return s.st.SetSetting(ctx, assistantKey, a)
}

// AssistantOn tells the interface whether to offer the assistant.
func (s *Service) AssistantOn(ctx context.Context) bool {
	a, _, err := s.AssistantSettings(ctx)
	return err == nil && a.Enabled
}

// SetAssistantClient replaces the HTTP client towards the model (tests).
func (s *Service) SetAssistantClient(c *http.Client) { s.asst.client = c }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chat sends the conversation and returns the model's answer. With asJSON
// the model is asked for a JSON object (servers that do not know the
// option are asked again without it).
func (s *Service) chat(ctx context.Context, a AssistantSettings, messages []chatMessage, asJSON bool) (string, error) {
	key, err := s.secretSetting(ctx, assistantSecretKey)
	if err != nil {
		return "", err
	}
	client := s.asst.client
	if client == nil {
		client = &http.Client{Timeout: assistantTimeout}
	}
	// The answers are short: a bound stops a model that rambles.
	body := map[string]any{"model": a.Model, "messages": messages, "temperature": 0.2, "stream": false, "max_tokens": 700}
	if asJSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	for attempt := 0; ; attempt++ {
		raw, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL+"/chat/completions", bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("assistente non raggiungibile: %w", redactURL(err))
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusBadRequest && asJSON && attempt == 0 {
			delete(body, "response_format")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal(data, &e)
			return "", fmt.Errorf("l'assistente ha risposto %d %s", resp.StatusCode, firstLine(e.Error.Message, errors.New("")))
		}
		var out struct {
			Choices []struct {
				Message chatMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
			return "", ErrAssistant
		}
		return out.Choices[0].Message.Content, nil
	}
}

func redactURL(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}

// jsonObject finds the JSON object in an answer: models sometimes wrap it
// in text or in a code block.
func jsonObject(answer string, v any) error {
	i, j := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if i < 0 || j < i {
		return ErrAssistant
	}
	if err := json.Unmarshal([]byte(answer[i:j+1]), v); err != nil {
		return ErrAssistant
	}
	return nil
}

func (s *Service) assistantOn(ctx context.Context) (AssistantSettings, error) {
	a, _, err := s.AssistantSettings(ctx)
	if err != nil {
		return a, err
	}
	if !a.Enabled {
		return a, ErrAssistantOff
	}
	return a, nil
}

// TestAssistant asks the model for a short answer and says how long it took.
func (s *Service) TestAssistant(ctx context.Context) (string, time.Duration, error) {
	a, _, err := s.AssistantSettings(ctx)
	if err != nil {
		return "", 0, err
	}
	if a.URL == "" || a.Model == "" {
		return "", 0, ErrAssistantOff
	}
	start := time.Now()
	answer, err := s.chat(ctx, a, []chatMessage{{Role: "user", Content: "Reply with one short sentence: which model are you?"}}, false)
	return strings.TrimSpace(answer), time.Since(start), err
}

func languageName(ctx context.Context) string {
	if LangOf(ctx) == "en" {
		return "English"
	}
	return "Italian"
}

// ---- A recording rule from a sentence ----

// RuleProposal is a rule to check in the form, with the model's reasoning.
type RuleProposal struct {
	Rule        RuleInput `json:"rule"`
	ChannelName string    `json:"channel_name,omitempty"`
	Explanation string    `json:"explanation"`
}

const rulePrompt = `You turn a request into a recording rule for VodMax, a TV recorder. A rule records every programme of the TV guide whose title (or subtitle) matches. Fields:
- name: short name of the rule (max 40 characters).
- all: words that must ALL appear in the title, as they appear in TV guides (for example ["juventus"] or ["formula 1"]). Keep it to the essential, 1-3 words.
- any: optional, at least one of these must appear (for example ["gara", "qualifiche"]).
- none: optional, the programme is skipped if it contains one of these (for example ["highlights", "replica"]).
- channel: exact name of one channel from the list, only if the person names a channel; otherwise "".
- section: "sport", "tv" or "" (any).
- live_only: true when the person wants only live events, not reruns or delayed broadcasts.
- keep: how many recordings to keep (0 = all), only if the person says so.
- explanation: one sentence, in %s, saying what the rule will record.
Answer with only a JSON object with exactly these fields.

Channels: %s`

// AssistantRule proposes a rule for a sentence; nothing is saved.
func (s *Service) AssistantRule(ctx context.Context, text string) (RuleProposal, error) {
	var p RuleProposal
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 500 {
		return p, invalid("scrivi cosa registrare (max 500 caratteri)")
	}
	a, err := s.assistantOn(ctx)
	if err != nil {
		return p, err
	}
	v := viewerOf(ctx)
	channels, err := s.st.TVChannels(ctx, v.ProfileID, v.Kids, "", false, time.Now())
	if err != nil {
		return p, err
	}
	names := make([]string, 0, len(channels))
	for _, c := range channels {
		if c.Section != "radio" && len(names) < 400 {
			names = append(names, c.Name)
		}
	}
	answer, err := s.chat(ctx, a, []chatMessage{
		{Role: "system", Content: fmt.Sprintf(rulePrompt, languageName(ctx), strings.Join(names, "; "))},
		{Role: "user", Content: text + "\n(Explanation in " + languageName(ctx) + ".)"},
	}, true)
	if err != nil {
		return p, err
	}
	var out struct {
		Name        string   `json:"name"`
		All         []string `json:"all"`
		Any         []string `json:"any"`
		None        []string `json:"none"`
		Channel     string   `json:"channel"`
		Section     string   `json:"section"`
		LiveOnly    bool     `json:"live_only"`
		Keep        int      `json:"keep"`
		Explanation string   `json:"explanation"`
	}
	if err := jsonObject(answer, &out); err != nil {
		return p, err
	}
	p.Rule = RuleInput{Name: truncate(out.Name, 60), All: out.All, Any: out.Any, None: out.None, LiveOnly: out.LiveOnly, Keep: out.Keep}
	if out.Section == "sport" || out.Section == "tv" {
		p.Rule.Section = out.Section
	}
	if want := fold(out.Channel); want != "" {
		for _, c := range channels {
			if fold(c.Name) == want {
				p.Rule.ChannelID, p.ChannelName = c.ID, c.Name
				break
			}
		}
	}
	p.Explanation = truncate(strings.TrimSpace(out.Explanation), 300)
	// The same checks as a rule typed by hand.
	r, err := s.ruleFromInput(ctx, p.Rule)
	if err != nil {
		return p, err
	}
	p.Rule.Name, p.Rule.All, p.Rule.Any, p.Rule.None, p.Rule.Keep = r.Name, r.Spec.All, r.Spec.Any, r.Spec.None, r.Keep
	return p, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// ---- What to watch tonight ----

// Pick is one suggestion: a programme on TV (Channel...) or a title of the
// catalog (ItemID or Episode).
type Pick struct {
	Kind      string `json:"kind"` // tv, movie, series, episode
	Title     string `json:"title"`
	Why       string `json:"why"`
	ChannelID int64  `json:"channel_id,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Start     int64  `json:"start,omitempty"`
	Stop      int64  `json:"stop,omitempty"`
	ItemID    int64  `json:"item_id,omitempty"`
	Image     string `json:"image,omitempty"`
}

type Tonight struct {
	Intro string `json:"intro"`
	Picks []Pick `json:"picks"`
}

const tonightPrompt = `You help a person choose what to watch tonight. Below are the programmes on their TV channels (codes T…) and titles from their own library (codes L…): what they started, their list and what is trending. Choose up to 5 that fit them best, mixing TV and library when it makes sense; prefer what they seem to like. %s
Answer in %s with only a JSON object: {"intro": "one short sentence", "picks": [{"code": "T12", "why": "one short sentence"}]}. Use only codes from the lists.`

// AssistantTonight asks the model for tonight's picks among the viewer's
// programmes and titles; wish is what the person feels like, if anything.
func (s *Service) AssistantTonight(ctx context.Context, wish string) (Tonight, error) {
	var out Tonight
	wish = strings.TrimSpace(wish)
	if len(wish) > 200 {
		return out, invalid("richiesta troppo lunga (max 200 caratteri)")
	}
	a, err := s.assistantOn(ctx)
	if err != nil {
		return out, err
	}
	v := viewerOf(ctx)
	cacheKey := strconv.FormatInt(v.ProfileID, 10) + "\x00" + LangOf(ctx) + "\x00" + fold(wish)
	s.asst.mu.Lock()
	if e, ok := s.asst.tonight[cacheKey]; ok && time.Since(e.at) < tonightCache {
		s.asst.mu.Unlock()
		return e.out, nil
	}
	s.asst.mu.Unlock()

	tv, err := s.tonightProgrammes(ctx, v)
	if err != nil {
		return out, err
	}
	lib := s.tonightLibrary(ctx)
	if len(tv) == 0 && len(lib) == 0 {
		return out, invalid("niente da proporre: né guida TV per stasera né titoli nella libreria")
	}
	var list strings.Builder
	list.WriteString("TV tonight:\n")
	for i, p := range tv {
		fmt.Fprintf(&list, "T%d | %s %s | %s", i, time.Unix(p.Start, 0).Format("15:04"), p.Channel, p.Title)
		if p.extra != "" {
			list.WriteString(" | " + p.extra)
		}
		list.WriteString("\n")
	}
	list.WriteString("\nLibrary:\n")
	for i, p := range lib {
		fmt.Fprintf(&list, "L%d | %s | %s\n", i, p.Kind, p.extra)
	}
	note := ""
	if wish != "" {
		note = "They said: \"" + wish + "\"."
	}
	if v.Kids {
		note += " This is a children's profile: only suggest what suits children."
	}
	answer, err := s.chat(ctx, a, []chatMessage{
		{Role: "system", Content: fmt.Sprintf(tonightPrompt, note, languageName(ctx))},
		// Small models forget the language asked at the top of a long list.
		{Role: "user", Content: list.String() + "\nAnswer in " + languageName(ctx) + "."},
	}, true)
	if err != nil {
		return out, err
	}
	var raw struct {
		Intro string `json:"intro"`
		Picks []struct {
			Code string `json:"code"`
			Why  string `json:"why"`
		} `json:"picks"`
	}
	if err := jsonObject(answer, &raw); err != nil {
		return out, err
	}
	out.Intro = truncate(strings.TrimSpace(raw.Intro), 200)
	seen := map[string]bool{} // codes and titles: the same film twice is one pick
	for _, r := range raw.Picks {
		code := strings.ToUpper(strings.TrimSpace(r.Code))
		if seen[code] || len(code) < 2 || len(out.Picks) == 5 {
			continue
		}
		n, err := strconv.Atoi(code[1:])
		if err != nil {
			continue
		}
		var p Pick
		switch {
		case code[0] == 'T' && n >= 0 && n < len(tv):
			p = tv[n].Pick
		case code[0] == 'L' && n >= 0 && n < len(lib):
			p = lib[n].Pick
		default:
			continue // a code the model made up
		}
		if seen["title:"+fold(p.Title)] {
			continue
		}
		seen[code], seen["title:"+fold(p.Title)] = true, true
		p.Why = truncate(strings.TrimSpace(r.Why), 200)
		out.Picks = append(out.Picks, p)
	}
	if len(out.Picks) == 0 {
		return out, ErrAssistant
	}
	s.asst.mu.Lock()
	if s.asst.tonight == nil {
		s.asst.tonight = map[string]tonightEntry{}
	}
	s.asst.tonight[cacheKey] = tonightEntry{at: time.Now(), out: out}
	s.asst.mu.Unlock()
	return out, nil
}

type candidate struct {
	Pick
	extra string // what the model reads besides the title
}

// tonightProgrammes: from now to the end of the evening (at least 3
// hours) on the viewer's channels; at most 60, those on air now and those
// of prime time first. With the library the list stays near 2000 tokens:
// within what small local models read by default (Ollama cuts the rest
// without a word), and about 40 seconds of reading on a processor.
func (s *Service) tonightProgrammes(ctx context.Context, v Viewer) ([]candidate, error) {
	now := time.Now()
	from := now
	to := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 30, 0, 0, now.Location())
	if to.Sub(from) < 3*time.Hour {
		to = from.Add(3 * time.Hour)
	}
	channels, err := s.st.TVChannels(ctx, v.ProfileID, v.Kids, "", false, now)
	if err != nil {
		return nil, err
	}
	s.withLogos(channels)
	byGuide := map[string]store.TVChannel{}
	var guides []string
	for _, c := range channels {
		if c.GuideID == "" || c.Section == "radio" {
			continue
		}
		if _, dup := byGuide[c.GuideID]; !dup {
			byGuide[c.GuideID] = c
			guides = append(guides, c.GuideID)
		}
	}
	progs, err := s.st.GuideWindow(ctx, guides, from, to)
	if err != nil {
		return nil, err
	}
	var out []candidate
	for g, list := range progs {
		c := byGuide[g]
		for _, p := range list {
			if p.Stop <= from.Unix() || p.Start >= to.Unix() || p.Stop-p.Start < 15*60 {
				continue
			}
			extra := p.Category
			if d := truncate(strings.Join(strings.Fields(p.Description), " "), 40); d != "" {
				extra = strings.TrimSpace(extra + " " + d)
			}
			if p.Airing == "live" {
				extra = "LIVE " + extra
			}
			out = append(out, candidate{Pick: Pick{Kind: "tv", Title: p.Title, ChannelID: c.ID, Channel: c.Name, Start: p.Start, Stop: p.Stop, Image: c.Logo}, extra: extra})
		}
	}
	prime := func(c candidate) bool {
		t := time.Unix(c.Start, 0)
		h := t.Hour()*60 + t.Minute()
		return c.Start <= now.Unix() || (h >= 20*60 && h <= 23*60+30)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := prime(out[i]), prime(out[j]); pi != pj {
			return pi
		}
		return out[i].Start < out[j].Start
	})
	if len(out) > 60 {
		out = out[:60]
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].Channel < out[j].Channel
	})
	return out, nil
}

// tonightLibrary: what the viewer started, their list and the week's
// trending titles, at most 20.
func (s *Service) tonightLibrary(ctx context.Context) []candidate {
	var out []candidate
	seen := map[int64]bool{}
	if cont, err := s.Continue(ctx); err == nil {
		for _, c := range cont {
			if len(out) >= 8 {
				break
			}
			if c.Media == "episode" {
				if seen[c.SeriesID] {
					continue
				}
				seen[c.SeriesID] = true
				title := fmt.Sprintf("%s S%dE%d", c.SeriesTitle, c.Season, c.Number)
				out = append(out, candidate{Pick: Pick{Kind: "series", Title: c.SeriesTitle, ItemID: c.SeriesID, Image: c.Logo}, extra: title + " (started)"})
			} else if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, candidate{Pick: Pick{Kind: "movie", Title: c.Title, ItemID: c.ID, Image: c.Logo}, extra: c.Title + " (started)"})
			}
		}
	}
	add := func(list []store.CatalogItem, why string) {
		for _, it := range list {
			if len(out) >= 20 || seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			extra := it.DisplayTitle
			if it.Year != nil {
				extra += fmt.Sprintf(" (%d)", *it.Year)
			}
			out = append(out, candidate{Pick: Pick{Kind: it.Kind, Title: it.DisplayTitle, ItemID: it.ID, Image: it.Logo}, extra: extra + " " + why})
		}
	}
	if mine, err := s.MyList(ctx); err == nil {
		add(mine, "(in their list)")
	}
	if trend, err := s.Trending(ctx); err == nil {
		add(trend, "(trending)")
	}
	return out
}
