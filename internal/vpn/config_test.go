package vpn

import (
	"fmt"
	"strings"
	"testing"
)

const sampleConf = `
[Interface]
# Example in the format used by Proton / Mullvad
PrivateKey = %s
Address = 10.2.0.2/32, fd00::2/128
DNS = 10.2.0.1, example.internal
PostUp = iptables -A FORWARD -j ACCEPT

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.com:51820
`

func TestParseConfig(t *testing.T) {
	priv, pub := testKeyPair(t)
	cfg, err := ParseConfig(strings.NewReader(fmt.Sprintf(sampleConf, b64(priv), b64(pub))))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKey != priv || cfg.PeerPublicKey != pub {
		t.Fatal("keys not parsed")
	}
	if len(cfg.Addresses) != 2 || len(cfg.DNS) != 1 || cfg.DNS[0].String() != "10.2.0.1" {
		t.Fatalf("addresses/dns: %v %v", cfg.Addresses, cfg.DNS)
	}
	if cfg.keepalive() != defaultKeepalive || cfg.mtu() != defaultMTU {
		t.Fatal("defaults not applied")
	}
	joined := strings.Join(cfg.Ignored, ",")
	if !strings.Contains(joined, "postup") || !strings.Contains(joined, "example.internal") {
		t.Fatalf("ignored keys not reported: %v", cfg.Ignored)
	}
}

func TestParseConfigRejects(t *testing.T) {
	priv, pub := testKeyPair(t)
	cases := map[string]string{
		"no dns":       strings.Replace(fmt.Sprintf(sampleConf, b64(priv), b64(pub)), "DNS = 10.2.0.1, example.internal", "", 1),
		"bad key":      fmt.Sprintf(sampleConf, "not-a-key", b64(pub)),
		"two peers":    fmt.Sprintf(sampleConf, b64(priv), b64(pub)) + "[Peer]\nPublicKey = " + b64(pub) + "\n",
		"unknown key":  fmt.Sprintf(sampleConf, b64(priv), b64(pub)) + "Foo = bar\n",
		"bad endpoint": strings.Replace(fmt.Sprintf(sampleConf, b64(priv), b64(pub)), "vpn.example.com:51820", "vpn.example.com", 1),
	}
	for name, conf := range cases {
		if _, err := ParseConfig(strings.NewReader(conf)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestKeysNeverPrinted(t *testing.T) {
	priv, pub := testKeyPair(t)
	cfg, err := ParseConfig(strings.NewReader(fmt.Sprintf(sampleConf, b64(priv), b64(pub))))
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{fmt.Sprint(cfg), fmt.Sprintf("%+v", *cfg), fmt.Sprintf("%v", cfg.PrivateKey)} {
		if strings.Contains(out, b64(priv)) || strings.Contains(out, priv.hex()) {
			t.Fatal("private key leaked in formatted output")
		}
	}
}
