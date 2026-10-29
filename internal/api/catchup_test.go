package api

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCatchupSessions(t *testing.T) {
	e, _ := tvSetup(t)
	tv := channels(t, e, "?section=tv")
	var rai, other int64
	for _, c := range tv {
		switch c.Name {
		case "Rai 1":
			rai = c.ID
		case "Canale 5":
			other = c.ID
		}
	}
	now := time.Now()
	ask := func(ch int64, start, stop time.Time) (int, string) {
		code, body := e.do(t, "POST", "/api/v1/catchup/"+strconv.FormatInt(ch, 10)+"/session",
			map[string]any{"viewer": "salotto-tv-01", "start": start.Unix(), "stop": stop.Unix(), "title": "TG1"}, true)
		return code, string(body)
	}
	if code, body := ask(rai, now.Add(-2*time.Hour), now.Add(-time.Hour)); code != 200 ||
		!strings.Contains(body, `"mode":"remux"`) || !strings.Contains(body, `"duration":3600`) || !strings.Contains(body, "TG1 · Rai 1") {
		t.Fatalf("archive: %d %s", code, body)
	}
	// On air: up to now.
	if code, body := ask(rai, now.Add(-30*time.Minute), now.Add(30*time.Minute)); code != 200 || !strings.Contains(body, `"duration":1800`) {
		t.Fatalf("on air: %d %s", code, body)
	}
	for _, c := range []struct {
		name        string
		ch          int64
		start, stop time.Time
		want        string
	}{
		{"no archive", other, now.Add(-2 * time.Hour), now.Add(-time.Hour), "non ha l'archivio"},
		{"too old", rai, now.AddDate(0, 0, -4), now.AddDate(0, 0, -4).Add(time.Hour), "3 giorni"},
		{"future", rai, now.Add(time.Hour), now.Add(2 * time.Hour), "non è ancora iniziato"},
		{"too long", rai, now.Add(-10 * time.Hour), now.Add(-time.Hour), "massimo 6 ore"},
	} {
		if code, body := ask(c.ch, c.start, c.stop); code != 400 || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s", c.name, code, body)
		}
	}
}
