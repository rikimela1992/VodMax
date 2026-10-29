package api

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExternalGuideFillsGaps(t *testing.T) {
	e, _ := tvSetup(t)
	now := time.Now().UTC()
	f := func(d time.Duration) string { return now.Add(d).Format("20060102150405 +0000") }
	xml := fmt.Sprintf(`<?xml version="1.0"?><tv>
		<channel id="Rai1.it"><display-name>Rai 1</display-name></channel>
		<channel id="alma.ext"><display-name>Alma TV</display-name></channel>
		<channel id="cielo.ext"><display-name>Cielo</display-name></channel>
		<channel id="altro.ext"><display-name>Canale che non c'è</display-name></channel>
		<programme start="%s" stop="%s" channel="Rai1.it"><title>NON DEVE ENTRARE</title></programme>
		<programme start="%s" stop="%s" channel="Rai1.it"><title>Estensione</title></programme>
		<programme start="%s" stop="%s" channel="alma.ext"><title>Cucina con Alma</title></programme>
		<programme start="%s" stop="%s" channel="alma.ext"><title>Viaggi</title></programme>
		<programme start="%s" stop="%s" channel="cielo.ext"><title>NEMMENO QUESTO</title></programme>
		<programme start="%s" stop="%s" channel="altro.ext"><title>Altro</title></programme>
		</tv>`, f(-time.Hour), f(time.Hour), f(3*time.Hour), f(4*time.Hour), f(-30*time.Minute), f(30*time.Minute), f(30*time.Minute), f(26*time.Hour),
		f(-time.Hour), f(time.Hour), f(-time.Hour), f(time.Hour))
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(xml))
	zw.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/guida.xml.gz" {
			w.WriteHeader(404)
			return
		}
		w.Write(gz.Bytes())
	}))
	defer src.Close()

	if code, _ := e.do(t, "POST", "/api/v1/guide/sources", map[string]string{"name": "EPG", "url": "ftp://x"}, true); code != 400 {
		t.Fatal("ftp accepted")
	}
	e.do(t, "POST", "/api/v1/guide/sources", map[string]string{"name": "EPG", "url": src.URL + "/guida.xml.gz"}, true)
	code, body := e.do(t, "POST", "/api/v1/guide/sources/1/sync", nil, true)
	if code != 200 || !strings.Contains(string(body), `"channels":2`) || !strings.Contains(string(body), `"programmes":3`) {
		t.Fatalf("sync: %d %s", code, body)
	}
	titles := func() map[string][]string {
		out := map[string][]string{}
		for _, r := range readGuide(t, e, "?hours=12&section=tv") {
			for _, p := range r.Programmes {
				out[r.Channel.Name] = append(out[r.Channel.Name], p.Title)
			}
		}
		return out
	}
	got := titles()
	if strings.Join(got["Alma Tv"], ",") != "Cucina con Alma,Viaggi" || strings.Contains(fmt.Sprint(got), "NON DEVE") ||
		strings.Contains(fmt.Sprint(got), "NEMMENO") || strings.Join(got["Rai 1"], ",") != "TG1,Film della sera,Estensione" {
		t.Fatalf("guide: %v", got)
	}
	// The provider guide is refreshed: the external programmes stay.
	e.do(t, "POST", "/api/v1/subscriptions/1/guide/sync", nil, true)
	if got := titles(); len(got["Alma Tv"]) != 2 || len(got["Rai 1"]) != 3 {
		t.Fatalf("after provider refresh: %v", got)
	}
	_, body = e.do(t, "GET", "/api/v1/guide/sources", nil, true)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("sources: %s", body)
	}
	// Disabled: its programmes go.
	e.do(t, "PATCH", "/api/v1/guide/sources/1", map[string]any{"enabled": false}, true)
	if got := titles(); len(got["Alma Tv"]) != 0 || len(got["Rai 1"]) != 2 {
		t.Fatalf("after disabling: %v", got)
	}
	// A broken source is reported.
	e.do(t, "POST", "/api/v1/guide/sources", map[string]string{"name": "Rotta", "url": src.URL + "/manca.xml"}, true)
	if code, _ := e.do(t, "POST", "/api/v1/guide/sources/2/sync", nil, true); code == 200 {
		t.Fatal("missing guide synced")
	}
	if _, body := e.do(t, "GET", "/api/v1/status", nil, true); !strings.Contains(string(body), "Rotta") {
		t.Fatalf("status: %s", body)
	}
}
