package service

import (
	"context"
	"testing"
	"time"

	"github.com/silirim/vodmax/internal/store"
)

func TestAiringOf(t *testing.T) {
	ctx := WithLang(context.Background(), "it")
	now := time.Date(2026, 10, 30, 20, 45, 0, 0, time.Local)
	at := func(h int) int64 { return now.Add(time.Duration(h) * time.Hour).Unix() }
	prog := func(title, desc, airing string, h int) store.Programme {
		return store.Programme{Title: title, Description: desc, Airing: airing, Start: at(h), Stop: at(h + 2)}
	}
	ix := airingIndex{}
	first := prog("Serie A: Juventus - Inter", "", "", -26)
	ix.add(first, "Sky Sport Calcio")
	ix.add(first, "DAZN 1")       // the same airing on another channel
	for h := -20; h < 0; h += 5 { // reruns do not make it a title of every day
		ix.add(prog("Serie A: Juventus - Inter", "", "", h), "Sport 3")
	}
	ix.add(prog("LIVE: F1 Paddock Live", "", "", -24), "Sky Sport F1")
	ix.add(prog("LIVE: F1 Paddock Live", "", "", 0), "Sky Sport F1")
	for i := 0; i < 8; i++ {
		ix.add(prog("Calcio Serie A", "", "", -24*i-3), "Sport 1")
	}
	cases := []struct {
		p       store.Programme
		channel string
		want    string
	}{
		{prog("Juventus - Inter", "", "live", 0), "Sport 1", "live"},
		{prog("Juventus - Inter", "", "rerun", 0), "Sport 1", "rerun"},
		{prog("Juventus - Inter (replica)", "", "", 0), "Sport 1", "rerun"},
		{prog("Highlights: Juventus - Inter", "", "", 0), "Sport 1", "rerun"},
		{prog("Juventus - Inter", "Partita giocata ieri, in differita.", "", 0), "Sport 1", "rerun"},
		{prog("Juventus - Inter 2006", "", "", 0), "Sport 1", "rerun"},
		{prog("Serie A 2025/26: Juventus - Inter", "", "", 0), "Sport 1", ""}, // last season is still current
		{prog("Juventus - Inter", "", "", 0), "Sport 1 +1", "rerun"},
		{prog("Juventus - Inter", "", "", 0), "Inter Classic", "rerun"},
		{prog("Serie A: Juventus - Inter", "", "", 0), "Sport 2", "rerun"}, // on air last night
		{prog("Serie A: Juventus - Inter", "", "", -25), "Sport 2", ""},    // overlaps: one event on two channels
		{prog("Calcio Serie A", "", "", 0), "Sport 1", ""},                 // a title of every day says nothing
		{prog("Liverpool - Chelsea", "", "", 0), "Sport 1", ""},            // "live" is a whole word
		{prog("Liverpool - Chelsea", "In diretta da Anfield", "", 0), "Sport 1", "live"},
		{prog("Juventus - Inter", "", "premiere", 0), "Sport 1", "live"},
		{prog("Juventus - Inter", "Replica della diretta di ieri", "", 0), "Sport 1", "rerun"},
		// A show live every day under the same name; its rerun drops "LIVE:".
		{prog("LIVE: F1 Paddock Live", "", "", 0), "Sky Sport F1", "live"},
		{prog("F1 Paddock Live", "", "", 3), "Sky Sport F1", "rerun"},
	}
	for _, c := range cases {
		a := airingOf(ctx, c.p, c.channel, ix)
		if a.Kind != c.want {
			t.Errorf("%q %q on %s = %q (%s), want %q", c.p.Title, c.p.Description, c.channel, a.Kind, a.Why, c.want)
		}
		if a.Kind != "" && a.Why == "" {
			t.Errorf("%q: no reason", c.p.Title)
		}
	}
}
