package lineup

import "testing"

func TestMatch(t *testing.T) {
	l := Italy()
	cases := map[string]int{
		"Rai 1 HD": 1, "Rai 1 4K": 1, "RAI 1": 1, "Rai 3 TGR Regione": 3, "Rete 4 FHD": 4, "Canale 5 HD": 5,
		"IT: Canale 5": 5, "Italia 1 HD": 6, "La7 HD": 7, "Tv8 HD": 8, "Discovery Nove HD": 9, "Rai 4 HD": 21,
		"Iris HD": 22, "Cielo FHD": 26, "Cine34 HD": 34, "Boing HD": 40, "Super!": 47, "Sky TG24 FHD": 50,
		"TGcom 24 HD": 51, "Rai News 24 HD": 48, "Sportitalia HD": 60, "RTL 102.5": 36,
		"Discovery Real Time FHD": 31, "Discovery Dmax HD": 52, "Discovery Giallo HD": 38, "Discovery Home & Garden HGTV": 56, "Rai Sport + HD": 58, "Mediaset Twentyseven HD": 27, "Si Sport Italia HD": 60,
	}
	for name, want := range cases {
		c, ok := l.Match(name)
		if !ok || c.Number != want {
			t.Errorf("%q → %v %v, atteso %d", name, c.Number, ok, want)
		}
	}
	for _, name := range []string{"Canale 5 +1 HD", "Italia 1 +1", "Rai Italia", "Sky Uno HD", "Italia 7 Sport", "Sky Cinema Uno +24 HD"} {
		if c, ok := l.Match(name); ok {
			t.Errorf("%q non dovrebbe essere riconosciuto (→ %d)", name, c.Number)
		}
	}
}
