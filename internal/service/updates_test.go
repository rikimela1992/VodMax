package service

import "testing"

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.3.2", "v0.3.1", true},
		{"v0.4.0", "0.3.9", true},
		{"v0.3.1", "v0.3.1", false},
		{"v0.3.1", "v0.3.1-2-gabc123", false}, // a build after the tag is newer
		{"v0.3.2", "v0.3.1-2-gabc123-dirty", true},
		{"v0.4.0", "v0.4.0-rc1", true}, // the release after its candidate
		{"v1.0.0", "dev", false},       // unknown current version: no claim
		{"latest", "v0.1.0", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
