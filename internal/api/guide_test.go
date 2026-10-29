package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type guideRow struct {
	Channel struct {
		ID   int64
		Name string
	}
	Programmes []struct {
		Start, Stop int64
		Title       string
		Recording   int64
	}
}

func readGuide(t *testing.T, e *env, query string) []guideRow {
	t.Helper()
	code, body := e.do(t, "GET", "/api/v1/guide"+query, nil, true)
	var rows []guideRow
	if code != 200 || json.Unmarshal(body, &rows) != nil {
		t.Fatalf("guide: %d %s", code, body)
	}
	return rows
}

func TestGuideGridAndRecordingFromGuide(t *testing.T) {
	e, _ := tvSetup(t)
	rows := readGuide(t, e, "?hours=3&section=tv")
	var rai, cielo *guideRow
	for i := range rows {
		switch rows[i].Channel.Name {
		case "Rai 1":
			rai = &rows[i]
		case "Cielo":
			cielo = &rows[i]
		}
	}
	if rai == nil || len(rai.Programmes) != 2 || rai.Programmes[1].Title != "Film della sera" || cielo == nil || len(cielo.Programmes) != 1 {
		t.Fatalf("grid: %+v", rows)
	}
	// A window later than the guide: channels without programmes.
	later := readGuide(t, e, "?hours=2&section=tv&from="+itoa(time.Now().Add(5*time.Hour).Unix()))
	for _, r := range later {
		if len(r.Programmes) != 0 {
			t.Fatalf("programmes outside the window: %+v", r)
		}
	}

	// Record the evening film from the grid: margins added, marked in the grid.
	film := rai.Programmes[1]
	code, body := e.do(t, "POST", "/api/v1/recordings", map[string]any{"channel_id": rai.Channel.ID, "start": film.Start,
		"stop": film.Stop, "title": film.Title, "from_guide": true}, true)
	var rec struct{ ID, Start, Stop int64 }
	if code != 201 || json.Unmarshal(body, &rec) != nil || rec.Start != film.Start-120 || rec.Stop != film.Stop+300 {
		t.Fatalf("record from guide: %d %s", code, body)
	}
	rows = readGuide(t, e, "?hours=3&section=tv")
	for _, r := range rows {
		if r.Channel.Name == "Rai 1" && r.Programmes[1].Recording != rec.ID {
			t.Fatalf("recording not marked: %+v", r.Programmes)
		}
	}

	// One connection: another channel at the same time is refused, with
	// the reason.
	code, body = e.do(t, "POST", "/api/v1/recordings", map[string]any{"channel_id": cielo.Channel.ID, "start": film.Start + 600,
		"stop": film.Stop - 600, "title": "Altro"}, true)
	if code != 400 || !strings.Contains(string(body), "1 connessione") || !strings.Contains(string(body), "Film della sera") {
		t.Fatalf("conflict: %d %s", code, body)
	}
	// After it: fine.
	if code, body := e.do(t, "POST", "/api/v1/recordings", map[string]any{"channel_id": cielo.Channel.ID, "start": film.Stop + 600,
		"stop": film.Stop + 1800, "title": "Dopo"}, true); code != 201 {
		t.Fatalf("after: %d %s", code, body)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
