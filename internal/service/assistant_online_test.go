package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// toServer sends every request to srv, whatever the host.
type toServer struct{ srv *httptest.Server }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	u, _ := url.Parse(t.srv.URL)
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(r)
}

func TestClaudeOnline(t *testing.T) {
	var got map[string]any
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages" {
			key = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/messages":
			raw, _ := io.ReadAll(r.Body)
			json.Unmarshal(raw, &got)
			fmt.Fprint(w, `{"id":"m","type":"message","role":"assistant","model":"claude-haiku-5-5","stop_reason":"end_turn",
				"content":[{"type":"text","text":"Sono Claude."}],"usage":{"input_tokens":5,"output_tokens":3}}`)
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"claude-haiku-5-5","type":"model"},{"id":"claude-sonnet-5-5","type":"model"}],"has_more":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s, _ := localTestService(t)
	s.SetAssistantClient(&http.Client{Transport: toServer{srv}})
	ctx := context.Background()
	if err := s.SetAssistant(ctx, AssistantInput{Enabled: true, Mode: "online", Provider: "anthropic"}); err == nil {
		t.Fatal("turned on without a key")
	}
	k := "sk-test"
	if err := s.SetAssistant(ctx, AssistantInput{Enabled: true, Mode: "online", Provider: "anthropic", Key: &k}); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.AssistantSettings(ctx)
	if a.Model != "claude-haiku-5-5" {
		t.Fatalf("default model %q", a.Model)
	}
	answer, _, err := s.TestAssistant(ctx)
	if err != nil || answer != "Sono Claude." {
		t.Fatalf("answer %q, %v", answer, err)
	}
	if key != "Bearer sk-test" || got["model"] != "claude-haiku-5-5" {
		t.Fatalf("request: key %q, body %v", key, got)
	}
	if oc, _ := got["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Fatalf("effort: %v", got["output_config"])
	}
	ids, err := s.AssistantModels(ctx, "online", "anthropic", "", "")
	if err != nil || len(ids) != 2 {
		t.Fatalf("models %v, %v", ids, err)
	}
}
