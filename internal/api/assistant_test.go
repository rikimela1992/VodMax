package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeModel is an OpenAI-compatible chat server with a scripted answer.
type fakeModel struct {
	*httptest.Server
	mu       sync.Mutex
	answer   func(system, user string) string
	requests []map[string]any
	noJSON   bool // answers 400 to response_format, like some servers
}

func newFakeModel(t *testing.T) *fakeModel {
	m := &fakeModel{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"bad key"}}`))
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		m.mu.Lock()
		m.requests = append(m.requests, req)
		m.mu.Unlock()
		if _, ok := req["response_format"]; ok && m.noJSON {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"response_format not supported"}}`))
			return
		}
		msgs := req["messages"].([]any)
		system, user := "", ""
		for _, x := range msgs {
			mm := x.(map[string]any)
			if mm["role"] == "system" {
				system = mm["content"].(string)
			} else {
				user = mm["content"].(string)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": m.answer(system, user)}}}})
	}))
	t.Cleanup(m.Close)
	return m
}

func TestAssistant(t *testing.T) {
	e, _ := tvSetup(t)
	m := newFakeModel(t)
	if code, _ := e.do(t, "POST", "/api/v1/assistant/rule", map[string]string{"text": "la juve"}, true); code != 409 {
		t.Fatalf("off: %d", code)
	}
	if code, body := e.do(t, "GET", "/api/v1/assistant", nil, true); code != 200 || !strings.Contains(string(body), "false") {
		t.Fatalf("on: %d %s", code, body)
	}
	cfg := map[string]any{"enabled": true, "url": m.URL + "/v1/chat/completions", "model": "llama3", "key": "sk-test"}
	code, body := e.do(t, "PUT", "/api/v1/settings/assistant", cfg, true)
	if code != 200 || strings.Contains(string(body), "sk-test") || !strings.Contains(string(body), `"url":"`+m.URL+`/v1"`) {
		t.Fatalf("settings: %d %s", code, body)
	}
	m.answer = func(system, user string) string { return "I am llama3." }
	if code, body := e.do(t, "POST", "/api/v1/settings/assistant/test", nil, true); code != 200 || !strings.Contains(string(body), "llama3") {
		t.Fatalf("test: %d %s", code, body)
	}

	// A rule from a sentence: the model's channel name becomes the channel.
	m.noJSON = true
	m.answer = func(system, user string) string {
		if !strings.Contains(system, "Cielo") || user != "registra le partite della Juve in diretta su Cielo" {
			return "?"
		}
		return "Ecco la regola:\n```json\n" + `{"name":"Juventus","all":["juventus"],"any":[],"none":["highlights"],"channel":"cielo","section":"","live_only":true,"keep":0,"explanation":"Registra le partite della Juventus in diretta su Cielo."}` + "\n```"
	}
	code, body = e.do(t, "POST", "/api/v1/assistant/rule", map[string]string{"text": "registra le partite della Juve in diretta su Cielo"}, true)
	var p struct {
		Rule struct {
			Name      string
			All, None []string
			ChannelID int64 `json:"channel_id"`
			LiveOnly  bool  `json:"live_only"`
		}
		ChannelName string `json:"channel_name"`
		Explanation string
	}
	if code != 200 || json.Unmarshal(body, &p) != nil || p.Rule.Name != "Juventus" || p.Rule.All[0] != "juventus" || !p.Rule.LiveOnly ||
		p.Rule.ChannelID == 0 || p.ChannelName != "Cielo" || p.Explanation == "" {
		t.Fatalf("rule: %d %s", code, body)
	}
	if n := len(m.requests); n < 2 {
		t.Fatalf("no retry without response_format: %d requests", n)
	}
	// Not saved: the person checks it in the form.
	if _, body := e.do(t, "GET", "/api/v1/recording-rules", nil, true); strings.Contains(string(body), "Juventus") {
		t.Fatalf("saved: %s", body)
	}
	m.answer = func(system, user string) string { return "non so" }
	if code, _ := e.do(t, "POST", "/api/v1/assistant/rule", map[string]string{"text": "boh"}, true); code != 502 {
		t.Fatalf("unusable answer: %d", code)
	}

	// Tonight: only codes VodMax gave count.
	m.noJSON = false
	m.answer = func(system, user string) string {
		code := ""
		for _, line := range strings.Split(user, "\n") {
			if strings.Contains(line, "Film della sera") {
				code = strings.TrimSpace(strings.SplitN(line, "|", 2)[0])
			}
		}
		return `{"intro":"Stasera un film.","picks":[{"code":"` + code + `","why":"Ti piacciono i film."},{"code":"T999","why":"inventato"},{"code":"X1","why":"?"}]}`
	}
	code, body = e.do(t, "POST", "/api/v1/assistant/tonight", map[string]string{"wish": "qualcosa di leggero"}, true)
	var tn struct {
		Intro string
		Picks []struct {
			Kind, Title, Why, Channel string
			ChannelID                 int64 `json:"channel_id"`
			Start                     int64
		}
	}
	if code != 200 || json.Unmarshal(body, &tn) != nil || len(tn.Picks) != 1 || tn.Picks[0].Title != "Film della sera" ||
		tn.Picks[0].Channel == "" || tn.Picks[0].ChannelID == 0 || tn.Picks[0].Start == 0 || tn.Intro == "" {
		t.Fatalf("tonight: %d %s", code, body)
	}
	last := m.requests[len(m.requests)-1]
	if msgs := last["messages"].([]any); !strings.Contains(msgs[0].(map[string]any)["content"].(string), "qualcosa di leggero") {
		t.Fatalf("wish not passed: %v", msgs[0])
	}
	// The same question again within minutes: from the cache.
	n := len(m.requests)
	e.do(t, "POST", "/api/v1/assistant/tonight", map[string]string{"wish": "qualcosa di leggero"}, true)
	if len(m.requests) != n {
		t.Fatal("not cached")
	}
}
