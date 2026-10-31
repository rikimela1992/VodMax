package service

import (
	"testing"
	"time"
)

func TestTonightWindow(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, loc) }
	for _, c := range []struct{ now, from, to time.Time }{
		{at(31, 9, 25), at(31, 20, 0), at(32, 0, 30)},   // morning: the evening ahead, not the curling on now
		{at(31, 17, 59), at(31, 20, 0), at(32, 0, 30)},  // still before the evening
		{at(31, 18, 0), at(31, 18, 0), at(32, 0, 30)},   // from now on
		{at(31, 22, 45), at(31, 22, 45), at(32, 1, 45)}, // late: at least 3 hours
		{at(31, 1, 10), at(31, 1, 10), at(31, 4, 10)},   // after midnight: the night going on
	} {
		from, to := tonightWindow(c.now)
		if !from.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("%s: %s – %s, want %s – %s", c.now.Format("15:04"), from.Format("02 15:04"), to.Format("02 15:04"), c.from.Format("02 15:04"), c.to.Format("02 15:04"))
		}
	}
}

func TestHasPerson(t *testing.T) {
	for _, c := range []struct {
		people, person string
		want           bool
	}{
		{"Catherine Zeta-Jones, Michael Douglas", "Catherine Zeta-Jones", true},
		{"Catherine Zeta Jones", "Catherine Zeta-Jones", true},
		{"Brad Pitt, Eric Bana, Orlando Bloom Wolfgang Petersen", "Catherine Zeta-Jones", false},
		{"Leonardo DiCaprio Christopher Nolan", "Nolan", true},
		{"Kate Winslet", "Katherine Winslet", true}, // the given name may differ
		{"", "Brad Pitt", false},
	} {
		if got := hasPerson(c.people, c.person); got != c.want {
			t.Errorf("hasPerson(%q, %q) = %v", c.people, c.person, got)
		}
	}
}

func TestPeopleText(t *testing.T) {
	got := peopleText("Renée Zellweger, Richard Gere, Catherine Zeta-Jones", "Rob Marshall")
	if want := " renee zellweger , richard gere , catherine zeta jones , rob marshall "; got != want {
		t.Fatalf("peopleText = %q, want %q", got, want)
	}
	if peopleText("", "") != "" {
		t.Fatal("empty page")
	}
	for person, want := range map[string]string{"Catherine Zeta-Jones": "zeta jones", "catrin z jones": "z jones", "Nolan": "nolan"} {
		if got := personKey(person); got != want {
			t.Errorf("personKey(%q) = %q, want %q", person, got, want)
		}
	}
}
