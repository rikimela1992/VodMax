package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeFFmpeg stands in for ffmpeg's HLS output: it writes a playlist and
// a segment from what it reads, until its input ends.
func fakeFFmpeg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
for a; do last="$a"; done
dir=$(dirname "$last")
head -c 4000 > "$dir/seg000000.ts"
printf '#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg000000.ts\n' > "$dir/index.m3u8"
cat > /dev/null
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTimeshiftBuffer(t *testing.T) {
	e, p := tvSetup(t)
	e.svc.SetFFmpeg(fakeFFmpeg(t), "")
	dir := t.TempDir()
	if err := e.svc.SetTimeshiftDir(filepath.Join(dir, "ts"), nil); err != nil {
		t.Fatal(err)
	}
	if _, body := e.do(t, "GET", "/api/v1/settings/live", nil, true); !strings.Contains(string(body), `"mode":"timeshift"`) {
		t.Fatalf("default mode: %s", body)
	}
	tv := channels(t, e, "?section=tv")
	start := func(ch int64, viewer string, force bool) (int, []byte) {
		return e.do(t, "POST", "/api/v1/live/"+strconv.FormatInt(ch, 10)+"/timeshift", map[string]any{"viewer": viewer, "force": force}, true)
	}
	code, body := start(tv[0].ID, "salotto-tv-01", false)
	var sess struct{ ID, URL, KeepAlive, Stop string }
	if code != 200 || json.Unmarshal(body, &sess) != nil || !strings.HasSuffix(sess.URL, "/index.m3u8") {
		t.Fatalf("start: %d %s", code, body)
	}
	if code, ctype, data := get(t, e, sess.URL); code != 200 || ctype != "application/vnd.apple.mpegurl" || !strings.Contains(string(data), "seg000000.ts") {
		t.Fatalf("playlist: %d %s %s", code, ctype, data)
	}
	segURL := strings.TrimSuffix(sess.URL, "index.m3u8") + "seg000000.ts"
	if code, _, data := get(t, e, segURL); code != 200 || len(data) != 4000 {
		t.Fatalf("segment: %d %d", code, len(data))
	}
	for _, bad := range []string{"../../secret.key", "seg1.ts", "index.m3u8x"} {
		if code, _, _ := get(t, e, strings.TrimSuffix(sess.URL, "index.m3u8")+bad); code == 200 {
			t.Fatalf("served %q", bad)
		}
	}
	if code, _ := e.post(t, sess.KeepAlive); code != 204 {
		t.Fatalf("keepalive: %d", code)
	}
	// One provider connection: the viewer's channel change replaces its
	// buffer (no 409), the old buffer is gone.
	code, body = start(tv[1].ID, "salotto-tv-01", false)
	var next struct{ URL string }
	if code != 200 || json.Unmarshal(body, &next) != nil {
		t.Fatalf("zap: %d %s", code, body)
	}
	if code, _, _ := get(t, e, sess.URL); code != 403 {
		t.Fatalf("old buffer still served: %d", code)
	}
	// Another device: the connection is busy.
	if code, _ := start(tv[0].ID, "cucina-tv-0001", false); code != 409 {
		t.Fatalf("second device: %d", code)
	}
	// A film on the same device ends the buffer instead of fighting for
	// the connection.
	film := strconv.FormatInt(findItem(t, e, "movie", "1408"), 10)
	e.do(t, "POST", "/api/v1/vod/movie/"+film+"/session", map[string]string{"viewer": "salotto-tv-01"}, true)
	if code, _, _ := get(t, e, next.URL); code != 403 {
		t.Fatalf("buffer survived a film: %d", code)
	}
	// Leaving the page.
	code, body = start(tv[0].ID, "salotto-tv-01", false)
	json.Unmarshal(body, &sess)
	e.post(t, sess.Stop)
	if code, _, _ := get(t, e, sess.URL); code != 403 {
		t.Fatalf("stopped buffer served: %d", code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		entries, _ := os.ReadDir(filepath.Join(dir, "ts"))
		if len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("buffer files left: %v", entries)
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.mu.Lock()
	maxSeen := p.maxSeen
	p.mu.Unlock()
	if maxSeen > 1 {
		t.Fatalf("provider saw %d connections at once", maxSeen)
	}
	if code, _ := e.do(t, "PUT", "/api/v1/settings/live", map[string]any{"mode": "direct", "window_minutes": 5}, true); code != 400 {
		t.Fatal("5-minute window accepted")
	}
	if code, _ := e.do(t, "PUT", "/api/v1/settings/live", map[string]any{"mode": "direct", "window_minutes": 60}, true); code != 204 {
		t.Fatal("direct mode refused")
	}
}

func (e *env) post(t *testing.T, path string) (int, []byte) {
	t.Helper()
	code, _, body := func() (int, string, []byte) {
		resp, err := httpPost(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return resp.code, "", resp.body
	}()
	return code, body
}

type postResult struct {
	code int
	body []byte
}

func httpPost(url string) (postResult, error) {
	resp, err := http.Post(url, "text/plain", nil)
	if err != nil {
		return postResult{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return postResult{resp.StatusCode, b}, nil
}
