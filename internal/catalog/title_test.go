package catalog

import "testing"

func TestCleanTitle(t *testing.T) {
	cases := []struct {
		raw   string
		title string
		year  int
	}{
		{"1408 (2007)", "1408", 2007},
		{"Harry Potter e il calice di fuoco - 2005 4K", "Harry Potter e il calice di fuoco", 2005},
		{"Assassin's Creed (2016) 4K", "Assassin's Creed", 2016},
		{"Doctor Strange nel Multiverso della Follia 4K (2022)", "Doctor Strange nel Multiverso della Follia", 2022},
		{"Chainsaw Man (JP)", "Chainsaw Man", 0},
		{"[REC]³ - La genesi", "[REC]³ - La genesi", 0},
		{"Blade Runner 2049", "Blade Runner 2049", 0},
		{"1917", "1917", 0},
		{"(2019)", "(2019)", 0},
		{"Film [UHD_4K]", "Film", 0},
		{"Max Fleischer's Superman 1941-1942 (2009)", "Max Fleischer's Superman 1941-1942", 2009},
		{"1975 - L’anno che ha stravolto il mondo (2025)", "1975 - L’anno che ha stravolto il mondo", 2025},
	}
	for _, c := range cases {
		title, year := CleanTitle(c.raw)
		if title != c.title || year != c.year {
			t.Errorf("%q → (%q, %d), atteso (%q, %d)", c.raw, title, year, c.title, c.year)
		}
	}
}

func TestChannelHelpers(t *testing.T) {
	if got := ChannelTitle(`",US: ACC Network HD`); got != "US: ACC Network" {
		t.Errorf("ChannelTitle: %q", got)
	}
	for raw, want := range map[string]string{
		"Rai 1": "Rai 1", "Canale 5 4K": "Canale 5", "Canale 5 +1 HD": "Canale 5 +1", "Cielo FHD": "Cielo",
		"Adriano Celentano Channel ᴴᴰ": "Adriano Celentano Channel", "Sky Sport 24 HD": "Sky Sport 24", "HD": "HD",
	} {
		if got := ChannelTitle(raw); got != want {
			t.Errorf("ChannelTitle(%q) = %q, atteso %q", raw, got, want)
		}
	}
	for _, s := range []string{"----Italia----", "===== EAGLE CINEMA FULL HD =====", "## SPORT ##"} {
		if !IsSeparator(s) {
			t.Errorf("%q not detected as separator", s)
		}
	}
	for _, s := range []string{"Rai 1", "Italia 1 +1 HD", "C++ Cinema", "--Rai"} {
		if IsSeparator(s) {
			t.Errorf("%q wrongly detected as separator", s)
		}
	}
}
