package epg

import (
	"strings"
	"testing"
	"time"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="test">
  <channel id="Rai1.it"><display-name>Rai 1</display-name><display-name>RAI UNO</display-name></channel>
  <channel id="Canale5.it"><display-name>Canale 5</display-name></channel>
  <programme start="20261029200000 +0100" stop="20261029203000 +0100" channel="Rai1.it">
    <title lang="it">TG1</title><desc lang="it">Edizione delle 20.</desc><category>News</category>
  </programme>
  <programme start="20261029203000 +0100" stop="20261029230000 +0100" channel="Rai1.it">
    <title>Un film &amp; altro</title><sub-title>Prima visione</sub-title>
  </programme>
  <programme start="20261029200000 +0100" stop="20261029200000 +0100" channel="Rai1.it"><title>Durata zero</title></programme>
  <programme start="20261029200000 +0100" stop="20261029220000 +0100" channel="Canale5.it"><title>Scartato dal filtro</title></programme>
</tv>`

func TestParse(t *testing.T) {
	var seen []string
	g, err := Parse(strings.NewReader(sample), Options{
		OnChannel: func(c Channel) { seen = append(seen, c.ID) },
		Keep:      func(ch string, start, stop time.Time) bool { return ch == "rai1.it" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Channels) != 2 || g.Channels[0].ID != "rai1.it" || len(g.Channels[0].Names) != 2 || len(seen) != 2 {
		t.Fatalf("channels: %+v", g.Channels)
	}
	if len(g.Programmes) != 2 {
		t.Fatalf("programmes: %+v", g.Programmes)
	}
	p := g.Programmes[1]
	if p.Title != "Un film & altro" || p.Subtitle != "Prima visione" || p.Stop.Sub(p.Start) != 150*time.Minute {
		t.Fatalf("programme: %+v", p)
	}
	if g.Programmes[0].Start.UTC().Hour() != 19 {
		t.Fatalf("offset not applied: %v", g.Programmes[0].Start)
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{"20261029200000 +0100", "202610292000 +0100", "20261029190000"} {
		if tm, err := ParseTime(s); err != nil || tm.UTC().Hour() != 19 {
			t.Errorf("%q → %v %v", s, tm, err)
		}
	}
	if _, err := ParseTime("domani"); err == nil {
		t.Error("invalid time accepted")
	}
}
