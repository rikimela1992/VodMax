// Command vodmax-probe checks a WireGuard configuration and a provider
// account through VodMax's internal VPN, without the rest of the server.
//
//	vodmax-probe vpn-check   -conf wg.conf
//	vodmax-probe vpn-up      -conf wg.conf
//	vodmax-probe xtream-login -conf wg.conf -server URL -user NAME
//
// The Xtream password is read from VODMAX_XTREAM_PASSWORD so it never
// appears in the process list or shell history.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rikimela1992/vodmax/internal/playlist/xtream"
	"github.com/rikimela1992/vodmax/internal/provider"
	"github.com/rikimela1992/vodmax/internal/vpn"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	conf := fs.String("conf", "", "file .conf WireGuard")
	server := fs.String("server", "", "indirizzo del server Xtream")
	user := fs.String("user", "", "utente Xtream")
	wait := fs.Duration("wait", 15*time.Second, "attesa massima dell'handshake")
	fs.Parse(os.Args[2:])
	if *conf == "" {
		usage()
	}
	cfg := load(*conf)

	switch os.Args[1] {
	case "vpn-check":
		fmt.Printf("configurazione valida: indirizzi %v, DNS %v, endpoint %s\n", cfg.Addresses, cfg.DNS, cfg.Endpoint)
		if len(cfg.Ignored) > 0 {
			fmt.Printf("voci ignorate (non applicate): %v\n", cfg.Ignored)
		}
	case "vpn-up":
		tun := up(cfg, *wait)
		defer tun.Close()
		report(tun)
	case "xtream-login":
		password := os.Getenv("VODMAX_XTREAM_PASSWORD")
		if *server == "" || *user == "" || password == "" {
			fail(errors.New("servono -server, -user e VODMAX_XTREAM_PASSWORD"))
		}
		tun := up(cfg, *wait)
		defer tun.Close()
		report(tun)
		client, err := xtream.New(*server, *user, password, provider.NewHTTPClient(tun, 60*time.Second))
		if err != nil {
			fail(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		acct, err := client.Login(ctx)
		if err != nil {
			fail(err)
		}
		exp := "illimitato"
		if t := acct.Expires(); !t.IsZero() {
			exp = t.Format("2006-01-02")
		}
		fmt.Printf("account: stato %s, attivo %v, scadenza %s, connessioni %d/%d\n",
			acct.Status, acct.Active(), exp, acct.ActiveCons.Int(), acct.MaxConnections.Int())
		counts := map[string]func(context.Context) ([]xtream.Category, error){
			"categorie diretta": client.LiveCategories,
			"categorie film":    client.VODCategories,
			"categorie serie":   client.SeriesCategories,
		}
		for _, name := range []string{"categorie diretta", "categorie film", "categorie serie"} {
			cats, err := counts[name](ctx)
			if err != nil {
				fail(err)
			}
			fmt.Printf("%s: %d\n", name, len(cats))
		}
		report(tun)
	default:
		usage()
	}
}

func load(path string) *vpn.Config {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	cfg, err := vpn.ParseConfig(f)
	if err != nil {
		fail(fmt.Errorf("configurazione WireGuard: %w", err))
	}
	return cfg
}

func up(cfg *vpn.Config, wait time.Duration) *vpn.Tunnel {
	tun, err := vpn.Start(context.Background(), cfg, nil)
	if err != nil {
		fail(err)
	}
	deadline := time.Now().Add(wait)
	for !tun.Status().Connected {
		if time.Now().After(deadline) {
			tun.Close()
			fail(fmt.Errorf("nessun handshake con il server VPN entro %v", wait))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return tun
}

func report(tun *vpn.Tunnel) {
	st := tun.Status()
	fmt.Printf("VPN: collegata %v, ultimo handshake %s, ricevuti %d B, inviati %d B\n",
		st.Connected, st.LastHandshake.Format(time.TimeOnly), st.RxBytes, st.TxBytes)
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: vodmax-probe vpn-check|vpn-up|xtream-login -conf wg.conf [-server URL -user NOME]")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "errore:", err)
	os.Exit(1)
}
