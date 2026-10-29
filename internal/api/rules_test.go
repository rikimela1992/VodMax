package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

type ruleMatch struct {
	Channel, Title, State, Note string
}

func preview(t *testing.T, e *env, body map[string]any) []ruleMatch {
	t.Helper()
	code, out := e.do(t, "POST", "/api/v1/recording-rules/preview", body, true)
	var list []ruleMatch
	if code != 200 || json.Unmarshal(out, &list) != nil {
		t.Fatalf("preview: %d %s", code, out)
	}
	return list
}

func TestRecordingRules(t *testing.T) {
	e, _ := tvSetup(t)
	if code, _ := e.do(t, "POST", "/api/v1/recording-rules", map[string]any{"name": "Vuota", "all": []string{" "}}, true); code != 400 {
		t.Fatal("rule without words accepted")
	}
	film := map[string]any{"name": "Film", "all": []string{"film"}, "none": []string{"replica"}}
	if p := preview(t, e, film); len(p) != 1 || p[0].Channel != "Rai 1" || p[0].State != "new" {
		t.Fatalf("preview: %+v", p)
	}
	code, body := e.do(t, "POST", "/api/v1/recording-rules", film, true)
	var rule struct {
		ID       int64
		LastNote string `json:"last_note"`
	}
	if code != 201 || json.Unmarshal(body, &rule) != nil || !strings.Contains(rule.LastNote, "1 nuove") {
		t.Fatalf("add: %d %s", code, body)
	}
	recs := func() []struct {
		ID     int64
		Title  string
		RuleID int64 `json:"rule_id"`
	} {
		_, body := e.do(t, "GET", "/api/v1/recordings", nil, true)
		var list []struct {
			ID     int64
			Title  string
			RuleID int64 `json:"rule_id"`
		}
		json.Unmarshal(body, &list)
		return list
	}
	if l := recs(); len(l) != 1 || l[0].Title != "Film della sera" || l[0].RuleID != rule.ID {
		t.Fatalf("recordings: %+v", l)
	}
	// Applying again adds nothing.
	id := strconv.FormatInt(rule.ID, 10)
	e.do(t, "PUT", "/api/v1/recording-rules/"+id, film, true)
	if l := recs(); len(l) != 1 {
		t.Fatalf("duplicated: %+v", l)
	}
	// One connection: a programme on another channel at the same time.
	if p := preview(t, e, map[string]any{"name": "Case", "all": []string{"sogno"}}); len(p) != 1 || p[0].State != "conflict" ||
		!strings.Contains(p[0].Note, "Film della sera") {
		t.Fatalf("conflict preview: %+v", p)
	}
	// Cancelled by hand: not scheduled again.
	e.do(t, "DELETE", "/api/v1/recordings/"+strconv.FormatInt(recs()[0].ID, 10), nil, true)
	if l := recs(); len(l) != 0 {
		t.Fatalf("cancelled still listed: %+v", l)
	}
	_, body = e.do(t, "PUT", "/api/v1/recording-rules/"+id, film, true)
	if l := recs(); len(l) != 0 || !strings.Contains(string(body), "1 annullate a mano") {
		t.Fatalf("rescheduled after cancel: %+v %s", l, body)
	}
	if p := preview(t, e, film); len(p) != 1 || p[0].State != "cancelled" {
		t.Fatalf("cancelled preview: %+v", p)
	}
	// Kids profiles cannot manage rules.
	kid := addKids(t, e)
	if code, _ := e.call(t, "GET", "/api/v1/recording-rules", nil, kid.device, kid.profile); code != 403 {
		t.Fatalf("kids rules: %d", code)
	}
	// Space and limits.
	if code, body := e.do(t, "GET", "/api/v1/recordings/space", nil, true); code != 200 || !strings.Contains(string(body), "free_bytes") {
		t.Fatalf("space: %d %s", code, body)
	}
	if code, _ := e.do(t, "PUT", "/api/v1/settings/recordings", map[string]int{"max_gb": -1}, true); code != 400 {
		t.Fatal("negative limit accepted")
	}
	// Deleting the rule removes its markers.
	if code, _ := e.do(t, "DELETE", "/api/v1/recording-rules/"+id, nil, true); code != 204 {
		t.Fatal("delete rule")
	}
}

type kidsLogin struct{ device, profile string }

func addKids(t *testing.T, e *env) kidsLogin {
	t.Helper()
	_, body := e.do(t, "POST", "/api/v1/profiles", map[string]any{"name": "Bimbi", "kind": "kids"}, true)
	var p struct{ ID int64 }
	json.Unmarshal(body, &p)
	dev := pair(t, e, "Tablet", nil)
	_, tok := selectProfile(t, e, dev, p.ID, "")
	return kidsLogin{dev, tok}
}
