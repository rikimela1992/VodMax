package stalker

import "testing"

func TestPortalAddress(t *testing.T) {
	for in, want := range map[string]string{
		"http://p.example:8080/c/":                        "http://p.example:8080/portal.php",
		"http://p.example:8080/c":                         "http://p.example:8080/portal.php",
		"http://p.example/stalker_portal/c/":              "http://p.example/stalker_portal/server/load.php",
		"http://p.example/stalker_portal/server/load.php": "http://p.example/stalker_portal/server/load.php",
		"https://p.example/":                              "https://p.example/portal.php",
	} {
		c, err := New(in, "00:1a:79:00:00:01", nil)
		if err != nil || c.api != want {
			t.Errorf("%s → %v %v, want %s", in, c, err, want)
		}
	}
	for _, bad := range []string{"ftp://p.example/c/", "p.example/c/"} {
		if _, err := New(bad, "00:1A:79:00:00:01", nil); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	for _, bad := range []string{"00:1A:79:00:00", "zz:1A:79:00:00:01", ""} {
		if _, err := NormalizeMAC(bad); err == nil {
			t.Errorf("MAC %q accepted", bad)
		}
	}
}

func TestStreamURL(t *testing.T) {
	for in, want := range map[string]string{
		"ffmpeg http://s/1.ts?t=1": "http://s/1.ts?t=1",
		"auto https://s/live":      "https://s/live",
		"http://s/x":               "http://s/x",
		"ffmpeg rtmp://s/x":        "",
		"":                         "",
	} {
		if got := StreamURL(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
