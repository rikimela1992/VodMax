package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/silirim/vodmax/internal/store"
)

// A recording rule can ask for live broadcasts only: a match recorded
// should be the match being played, not last night's rerun or a classic
// of 2006. The guide rarely says so plainly, so airingOf weighs what it
// has: the flags of the XMLTV guide, words of the title and description,
// the years mentioned, the channel (timeshift "+1" channels, "Classic"
// channels) and whether the same programme was already on air.

// Airing is the guess for one programme: Kind is live, rerun or "" (no
// idea), Why says what it rests on.
type Airing struct {
	Kind string `json:"kind,omitempty"`
	Why  string `json:"why,omitempty"`
}

var (
	// Words of the title or subtitle that mark a rerun.
	rerunTitleWords = []string{"replica", "repliche", "differita", "differite", "highlights", "highlight", "sintesi", "best of", "amarcord",
		"rivivi", "rivivere", "rivediamo", "partita storica", "partite storiche", "match storico", "rerun", "replay", "repeat", "delayed"}
	// The description talks about the programme less exactly: only
	// phrases that say how it is broadcast.
	rerunDescWords = []string{"in differita", "replica della", "replica del", "gia trasmesso", "gia trasmessa", "registrata il",
		"registrato il", "rerun of", "repeat of"}
	liveWords = []string{"in diretta", "diretta", "live", "dal vivo", "en direct", "en vivo"}

	yearRE         = regexp.MustCompile(`\b(19[5-9][0-9]|20[0-9][0-9])\b`)
	rerunChannelRE = regexp.MustCompile(`(\+\s?[1-9]\b|\bclassics?\b|\bstory\b|\bstoria\b|\btimeshift\b)`)
)

// repeatGap is how far apart two airings of the same programme must
// start to be one programme and its rerun rather than one event on two
// channels; repeatWithin is how far back an earlier airing counts, and
// repeatSameDay how far for a title on air often ("Calcio Serie A" can
// be a different match every day).
const (
	repeatGap     = 90 * time.Minute
	repeatWithin  = 4 * 24 * time.Hour
	repeatSameDay = 20 * time.Hour
	oftenOnAir    = 6
)

// airingIndex lists when each programme was on air, to recognise reruns.
type airingIndex map[string][]aired

type aired struct {
	start, stop int64
	channel     string
	desc        string // folded
	live        bool   // said live in the title or by the guide
}

// A title says it is live at the start ("LIVE: F1 Paddock", "Diretta:
// Juventus - Inter") or at the end ("F4 - Gara 1 (Live)"), there only in
// brackets: "F1 Paddock Live" is the name of a show.
var (
	liveTitle  = regexp.MustCompile(`^(live|diretta|in diretta|en direct|en vivo) `)
	liveSuffix = regexp.MustCompile(`(?i)\s*[(\[](live|diretta|in diretta)[)\]]\s*$`)
)

// liveMarker is how the title says it is live, "" if it does not.
func liveMarker(title string) string {
	if m := liveSuffix.FindStringSubmatch(title); m != nil {
		return strings.ToLower(m[1])
	}
	return strings.TrimSpace(liveTitle.FindString(fold(title)))
}

// airingKey identifies a programme across airings: "LIVE: F1 Paddock" and
// its rerun "F1 Paddock" are the same.
func airingKey(p store.Programme) string {
	return liveTitle.ReplaceAllString(fold(liveSuffix.ReplaceAllString(p.Title, "")+" "+p.Subtitle), "")
}

func (ix airingIndex) add(p store.Programme, channel string) {
	k := airingKey(p)
	for _, a := range ix[k] {
		if a.start/1800 == p.Start/1800 {
			return // the same airing on another channel
		}
	}
	ix[k] = append(ix[k], aired{p.Start, p.Stop, channel, fold(p.Description), saidLive(p)})
}

func saidLive(p store.Programme) bool {
	return p.Airing == "live" || liveMarker(p.Title) != ""
}

// hasLive tells whether some airing of p is said live: then the guide
// marks the live ones, and the others are reruns.
func (ix airingIndex) hasLive(p store.Programme) bool {
	for _, a := range ix[airingKey(p)] {
		if a.live {
			return true
		}
	}
	return false
}

// earlier finds a previous airing of p: same title, and same description
// when both have one. A title on air often only counts within a day.
func (ix airingIndex) earlier(p store.Programme) *aired {
	list := ix[airingKey(p)]
	often := len(list) > oftenOnAir
	desc := fold(p.Description)
	for i := range list {
		a := &list[i]
		gap := time.Duration(p.Start-a.start) * time.Second
		switch {
		case a.stop > p.Start || gap < repeatGap || gap > repeatWithin:
		case a.desc != "" && desc != "" && a.desc != desc: // another event, same title
		case often && (gap > repeatSameDay || a.desc == "" || desc == ""):
		default:
			return a
		}
	}
	return nil
}

// firstWord finds the first of words in folded text, as whole words
// ("live" is not "Liverpool").
func firstWord(text string, words []string) string {
	for _, w := range words {
		if strings.Contains(" "+text+" ", " "+w+" ") {
			return w
		}
	}
	return ""
}

// airingOf guesses whether p, on channel, is live.
func airingOf(ctx context.Context, p store.Programme, channel string, ix airingIndex) Airing {
	switch p.Airing {
	case "live":
		return Airing{"live", T(ctx, "la guida la indica come diretta")}
	case "rerun":
		return Airing{"rerun", T(ctx, "la guida la indica come già trasmessa")}
	}
	title := fold(p.Title + " " + p.Subtitle)
	desc := fold(p.Description)
	if w := firstWord(title, rerunTitleWords); w != "" {
		return Airing{"rerun", T(ctx, "il titolo dice “%s”", w)}
	}
	if w := firstWord(desc, rerunDescWords); w != "" {
		return Airing{"rerun", T(ctx, "la descrizione dice “%s”", w)}
	}
	// A year before last in the title: a match of the past. Seasons
	// ("2025/26") count by their first year, so last year is still now.
	year := time.Unix(p.Start, 0).Year()
	latest := 0
	for _, y := range yearRE.FindAllString(p.Title+" "+p.Subtitle, -1) {
		n, _ := strconv.Atoi(y)
		latest = max(latest, n)
	}
	if latest > 0 && latest < year-1 {
		return Airing{"rerun", T(ctx, "il titolo parla del %d", latest)}
	}
	if m := rerunChannelRE.FindString(strings.ToLower(channel)); m != "" {
		return Airing{"rerun", T(ctx, "canale in differita o di repertorio (%s)", strings.TrimSpace(m))}
	}
	// Said in front of the title, live beats a title already on air:
	// a show that is live every day keeps its name.
	if m := liveMarker(p.Title); m != "" {
		return Airing{"live", T(ctx, "il titolo dice “%s”", m)}
	}
	if ix.hasLive(p) {
		return Airing{"rerun", T(ctx, "la diretta di questo programma ha “live” nel titolo, questa no")}
	}
	if a := ix.earlier(p); a != nil {
		return Airing{"rerun", T(ctx, "già in onda %s su %s", time.Unix(a.start, 0).Format("02/01 15:04"), a.channel)}
	}
	if w := firstWord(title+" "+desc, liveWords); w != "" {
		return Airing{"live", T(ctx, "la guida dice “%s”", w)}
	}
	if p.Airing == "premiere" || p.Airing == "new" {
		return Airing{"live", T(ctx, "prima visione secondo la guida")}
	}
	return Airing{}
}
