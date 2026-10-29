package api

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBackupRestoreOnNewInstallation(t *testing.T) {
	e, p := tvSetup(t)
	// User data: a favourite channel, film and episode progress, rules, a
	// category override, the schedule.
	tv := channels(t, e, "?section=tv")
	e.do(t, "PUT", "/api/v1/tv/favorites/"+strconv.FormatInt(tv[1].ID, 10), map[string]bool{"favorite": true}, true)
	film := strconv.FormatInt(findItem(t, e, "movie", "1408"), 10)
	e.do(t, "PUT", "/api/v1/progress/movie/"+film, map[string]int{"position": 1800, "duration": 6000}, true)
	series := strconv.FormatInt(findItem(t, e, "series", "twin"), 10)
	_, body := e.do(t, "GET", "/api/v1/series/"+series+"/episodes", nil, true)
	ep := strings.Split(strings.Split(string(body), `"id":`)[2], ",")[0]
	e.do(t, "PUT", "/api/v1/progress/episode/"+ep, map[string]int{"position": 600, "duration": 2800}, true)
	if code, _ := e.do(t, "PUT", "/api/v1/subscriptions/1/rules", map[string]any{"movie_min_year": 1990}, true); code != 200 {
		t.Fatal("rules")
	}
	e.do(t, "PUT", "/api/v1/settings/schedule", map[string]int{"catalog_hour": 2, "guide_hours": 3}, true)

	if code, _ := e.do(t, "POST", "/api/v1/backup", map[string]string{"password": "corta"}, true); code != 400 {
		t.Fatal("short password accepted")
	}
	code, file := e.do(t, "POST", "/api/v1/backup", map[string]string{"password": "una password lunga"}, true)
	if code != 200 || !bytes.HasPrefix(file, []byte("VODMAXB1P")) {
		t.Fatalf("backup: %d %q", code, file[:min(20, len(file))])
	}
	if bytes.Contains(file, []byte(p.URL)) || bytes.Contains(file, []byte("PrivateKey")) {
		t.Fatal("backup not encrypted")
	}

	// A new installation (other secret key, empty database).
	n := setup(t, true)
	if code, _ := restore(t, n, file, "sbagliata!!"); code != 400 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, body := restore(t, n, file, "una password lunga"); code != 204 {
		t.Fatalf("restore: %d %s", code, body)
	}
	// The catalog comes back in the background, then progress is attached.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body = n.do(t, "GET", "/api/v1/continue", nil, true)
		if strings.Contains(string(body), "1408") && strings.Contains(string(body), "Twin Peaks") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("progress not restored: %s", body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(string(body), `"position":1800`) || !strings.Contains(string(body), `"position":600`) {
		t.Fatalf("positions: %s", body)
	}
	if fav := channels(t, n, "?list=favorites"); len(fav) != 1 || fav[0].Name != tv[1].Name {
		t.Fatalf("favourites: %+v", fav)
	}
	if _, body = n.do(t, "GET", "/api/v1/subscriptions/1/rules", nil, true); !strings.Contains(string(body), `"movie_min_year":1990`) {
		t.Fatalf("rules: %s", body)
	}
	if _, body = n.do(t, "GET", "/api/v1/settings/schedule", nil, true); !strings.Contains(string(body), `"catalog_hour":2`) {
		t.Fatalf("schedule: %s", body)
	}
	// The provider password works (re-encrypted with the new key).
	if code, body := n.do(t, "POST", "/api/v1/subscriptions/1/sync", nil, true); code != 200 {
		t.Fatalf("sync after restore: %d %s", code, body)
	}
}

func TestAutomaticBackups(t *testing.T) {
	e, _ := tvSetup(t)
	if _, body := e.do(t, "GET", "/api/v1/backups", nil, true); string(bytes.TrimSpace(body)) != "[]" {
		t.Fatalf("list: %s", body)
	}
	code, body := e.do(t, "POST", "/api/v1/backups", nil, true)
	if code != 201 {
		t.Fatalf("make: %d %s", code, body)
	}
	name := strings.Split(strings.Split(string(body), `"name":"`)[1], `"`)[0]
	if code, _ := e.do(t, "GET", "/api/v1/backups/..%2Fsecret.key", nil, true); code != 404 {
		t.Fatalf("path traversal: %d", code)
	}
	e.do(t, "PUT", "/api/v1/subscriptions/1/rules", map[string]any{"movie_min_year": 2020}, true)
	if code, body := e.do(t, "POST", "/api/v1/backups/"+name+"/restore", nil, true); code != 204 {
		t.Fatalf("restore: %d %s", code, body)
	}
	if _, body := e.do(t, "GET", "/api/v1/subscriptions/1/rules", nil, true); strings.Contains(string(body), "2020") {
		t.Fatalf("rules not restored: %s", body)
	}
	// A copy made on another installation cannot be opened here.
	_, file := e.do(t, "GET", "/api/v1/backups/"+name, nil, true)
	other := setup(t, true)
	if code, _ := restore(t, other, file, ""); code != 400 {
		t.Fatalf("foreign automatic copy: %d", code)
	}
}

func restore(t *testing.T, e *env, file []byte, password string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/restore", bytes.NewReader(file))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Backup-Password", password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}
