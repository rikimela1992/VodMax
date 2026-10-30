package service

import (
	"testing"

	"github.com/silirim/vodmax/internal/store"
)

func TestRuleMatching(t *testing.T) {
	juve := store.RuleSpec{All: []string{"juve"}, None: []string{"highlights", "replica"}}
	f1 := store.RuleSpec{All: []string{"formula 1"}, Any: []string{"gara", "qualifiche"}}
	cases := []struct {
		spec  store.RuleSpec
		title string
		want  bool
	}{
		{juve, "Serie A: Juventus - Inter", true},
		{juve, "JUVENTÙS-Milan (diretta)", true},
		{juve, "Highlights: Juventus - Inter", false},
		{juve, "Il Mondo Juve - replica", false},
		{juve, "Napoli - Roma", false},
		{f1, "Formula 1 - GP d'Italia: Gara", true},
		{f1, "Formula 1 - GP d'Italia - Qualifiche", true},
		{f1, "Formula 1 - Prove libere 1", false},
		{f1, "Formula 10 - Gara", false},      // a phrase matches whole words
		{f1, "F1 GP del Messico: Gara", true}, // another name of the same thing
	}
	for _, c := range cases {
		if got := ruleMatches(c.spec, store.Programme{Title: c.title}); got != c.want {
			t.Errorf("%v on %q = %v", c.spec, c.title, got)
		}
	}
}
