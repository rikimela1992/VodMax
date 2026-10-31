package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func dictionary(t *testing.T, code string) map[string]string {
	t.Helper()
	raw, err := fs.ReadFile(static, "static/i18n/"+code+".json")
	if err != nil {
		t.Fatal(err)
	}
	d := map[string]string{}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s.json: %v", code, err)
	}
	return d
}

// keyRE finds t("…") and t("…" + "…") in the pages' scripts.
var keyRE = regexp.MustCompile(`\bt\(\s*((?:"(?:[^"\\]|\\.)*"\s*\+\s*)*"(?:[^"\\]|\\.)*")`)
var partRE = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// htmlTextRE finds an element with data-i18n: its attributes and its text
// up to the first tag.
var htmlTextRE = regexp.MustCompile(`<[a-z0-9]+(\s[^>]*\bdata-i18n\b[^>]*)>([^<]*)`)
var attrRE = regexp.MustCompile(`data-i18n-attr="([^"]*)"`)

// Every text of the interface is in the Italian base (what translators on
// Weblate see) and translated into English.
func TestEveryTextTranslated(t *testing.T) {
	it, en := dictionary(t, "it"), dictionary(t, "en")
	for k, v := range it {
		if k != v {
			t.Errorf("it.json: %q has the value %q", k, v)
		}
	}
	scripts, _ := fs.Glob(static, "static/*.js")
	for _, f := range scripts {
		if f == "static/i18n.js" {
			continue // the loader: its comments show t() as an example
		}
		src, _ := fs.ReadFile(static, f)
		for _, m := range keyRE.FindAllStringSubmatch(string(src), -1) {
			key := ""
			for _, p := range partRE.FindAllStringSubmatch(m[1], -1) {
				s, err := strconv.Unquote(`"` + p[1] + `"`)
				if err != nil {
					t.Fatalf("%s: %s", f, p[1])
				}
				key += s
			}
			if _, ok := it[key]; !ok {
				t.Errorf("%s: %q missing in it.json", f, key)
			}
			if en[key] == "" {
				t.Errorf("%s: %q not translated in en.json", f, key)
			}
		}
	}
	// Static text of the pages: data-i18n elements and their attributes.
	pages, _ := fs.Glob(static, "static/*.html")
	for _, f := range pages {
		src, _ := fs.ReadFile(static, f)
		for _, m := range htmlTextRE.FindAllStringSubmatch(string(src), -1) {
			texts := []string{strings.TrimSpace(m[2])}
			if a := attrRE.FindStringSubmatch(m[1]); a != nil {
				for _, name := range strings.Split(a[1], ",") {
					if v := regexp.MustCompile(`\s` + strings.TrimSpace(name) + `="([^"]*)"`).FindStringSubmatch(m[1]); v != nil {
						texts = append(texts, v[1])
					}
				}
			}
			for _, text := range texts {
				text = strings.NewReplacer("&amp;", "&", "&#39;", "'", "&quot;", `"`).Replace(text)
				if text != "" && en[text] == "" && text != "Multiview" && text != "Menu" {
					t.Errorf("%s: %q not translated in en.json", f, text)
				}
			}
		}
	}
	for k := range en {
		if _, ok := it[k]; !ok {
			t.Errorf("en.json: %q is not in it.json", k)
		}
	}
}

func TestLanguagesScript(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/i18n/languages.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body strings.Builder
	buf := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(buf)
		body.Write(buf[:n])
		if err != nil {
			break
		}
	}
	js, ok := strings.CutPrefix(body.String(), "window.VodMaxDicts = ")
	if resp.StatusCode != 200 || !ok || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("%d %s %.60s", resp.StatusCode, resp.Header.Get("Content-Type"), body.String())
	}
	var dicts map[string]map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(js), ";")), &dicts); err != nil {
		t.Fatal(err)
	}
	if _, it := dicts["it"]; it || dicts["en"]["Continua"] != "Continue" {
		t.Fatalf("languages: %d, it present: %v", len(dicts), it)
	}
}
