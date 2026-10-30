package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Errors come back in the language asked by X-VodMax-Lang, Italian
// without it.
func TestErrorLanguage(t *testing.T) {
	e := setup(t, true)
	post := func(lang string) (int, string) {
		raw, _ := json.Marshal(map[string]any{"name": "", "kind": "xtream", "vpn_id": 1})
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/subscriptions", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		if lang != "" {
			req.Header.Set("X-VodMax-Lang", lang)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var out struct{ Error string }
		json.Unmarshal(body, &out)
		return resp.StatusCode, out.Error
	}
	if code, msg := post("en"); code != 400 || msg != "subscription name required (max 80 characters)" {
		t.Fatalf("en: %d %q", code, msg)
	}
	if code, msg := post(""); code != 400 || msg != "nome dell'abbonamento obbligatorio (max 80 caratteri)" {
		t.Fatalf("it: %d %q", code, msg)
	}
	// A body that cannot be decoded: the format is translated, the
	// decoder's detail is kept.
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/subscriptions", bytes.NewReader([]byte("{")))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-VodMax-Lang", "en")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("invalid request body")) {
		t.Fatalf("decode: %s", body)
	}
}
