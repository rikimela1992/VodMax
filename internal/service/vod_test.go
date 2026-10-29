package service

import "testing"

func TestEpisodeTitle(t *testing.T) {
	for raw, want := range map[string]string{
		"Breaking Bad - Reazioni collaterali (2008) - S01E01 - Questione di chimica": "Questione di chimica",
		"Pilot":                   "Pilot",
		"Twin Peaks S02E01":       "Twin Peaks S02E01",
		"Serie s1e12: Il ritorno": "Il ritorno",
	} {
		if got := episodeTitle(raw); got != want {
			t.Errorf("%q → %q, atteso %q", raw, got, want)
		}
	}
}
