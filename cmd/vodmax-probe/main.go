// Command vodmax-probe checks a WireGuard configuration and a provider
// account through VodMax's internal VPN, without the rest of the server.
//
//	vodmax-probe vpn-check   -conf wg.conf
//	vodmax-probe vpn-up      -conf wg.conf
//	vodmax-probe xtream-login -conf wg.conf -server URL -user NAME
//	vodmax-probe xtream-dump  -conf wg.conf -server URL -user NAME
//
// xtream-dump writes the catalog structure as TSV (kind, category, name,
// year) to stdout: no URLs, no credentials. It is meant for designing
// filters on real provider data.
//
// The Xtream password is read from VODMAX_XTREAM_PASSWORD so it never
// appears in the process list or shell history.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
	channels := fs.String("channels", "", "nomi dei canali da provare, separati da virgola")
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
	case "xtream-dump":
		password := os.Getenv("VODMAX_XTREAM_PASSWORD")
		if *server == "" || *user == "" || password == "" {
			fail(errors.New("servono -server, -user e VODMAX_XTREAM_PASSWORD"))
		}
		tun := up(cfg, *wait)
		defer tun.Close()
		client, err := xtream.New(*server, *user, password, provider.NewHTTPClient(tun, 10*time.Minute))
		if err != nil {
			fail(err)
		}
		if err := dump(client); err != nil {
			fail(err)
		}
	case "xtream-live-check":
		password := os.Getenv("VODMAX_XTREAM_PASSWORD")
		if *server == "" || *user == "" || password == "" || *channels == "" {
			fail(errors.New("servono -server, -user, -channels e VODMAX_XTREAM_PASSWORD"))
		}
		tun := up(cfg, *wait)
		defer tun.Close()
		client, err := xtream.New(*server, *user, password, provider.NewHTTPClient(tun, 10*time.Minute))
		if err != nil {
			fail(err)
		}
		if err := liveCheck(client, strings.Split(*channels, ",")); err != nil {
			fail(err)
		}
		report(tun)
	case "xtream-vod-check":
		password := os.Getenv("VODMAX_XTREAM_PASSWORD")
		if *server == "" || *user == "" || password == "" {
			fail(errors.New("servono -server, -user e VODMAX_XTREAM_PASSWORD"))
		}
		tun := up(cfg, *wait)
		defer tun.Close()
		client, err := xtream.New(*server, *user, password, provider.NewHTTPClient(tun, 10*time.Minute))
		if err != nil {
			fail(err)
		}
		if err := vodCheck(client); err != nil {
			fail(err)
		}
	case "xtream-catchup-check":
		password := os.Getenv("VODMAX_XTREAM_PASSWORD")
		if *server == "" || *user == "" || password == "" {
			fail(errors.New("servono -server, -user e VODMAX_XTREAM_PASSWORD"))
		}
		tun := up(cfg, *wait)
		defer tun.Close()
		client, err := xtream.New(*server, *user, password, provider.NewHTTPClient(tun, 2*time.Minute))
		if err != nil {
			fail(err)
		}
		if err := catchupCheck(client, *channels); err != nil {
			fail(err)
		}
	default:
		usage()
	}
}

// catchupCheck reports the archive: how many channels have it, the server
// time zone, and whether an archive request returns MPEG-TS with each time
// zone interpretation. No URLs or credentials are printed.
func catchupCheck(c *xtream.Client, want string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, info, err := c.LoginInfo(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("fuso del server: %q, ora del server: %s\n", info.Timezone, info.TimeNow)
	live, err := c.LiveStreams(ctx)
	if err != nil {
		return err
	}
	days := map[int]int{}
	var pick *xtream.LiveStream
	for i, s := range live {
		if s.TVArchive.Int() == 1 {
			days[int(s.TVArchiveDuration.Int())]++
			if pick == nil && (want == "" || strings.Contains(strings.ToLower(s.Name.String()), strings.ToLower(want))) {
				pick = &live[i]
			}
		}
	}
	fmt.Printf("canali con archivio: %v (giorni → numero di canali) su %d\n", days, len(live))
	if pick == nil {
		return errors.New("nessun canale con archivio")
	}
	var names []string
	var picks []xtream.LiveStream
	for _, s := range live {
		if s.TVArchive.Int() == 1 {
			names = append(names, s.Name.String())
			if len(picks) < 3 {
				picks = append(picks, s)
			}
		}
	}
	fmt.Printf("con archivio: %s\n", strings.Join(names, " | "))
	start := time.Now().Add(-90 * time.Minute).Truncate(time.Minute)
	get := func(label, u string) {
		t0 := time.Now()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "VodMax")
		resp, err := c.HTTP().Do(req)
		if err != nil {
			fmt.Printf("  %-40s errore: %v\n", label, provider.Redact(err))
			return
		}
		buf := make([]byte, 188*20)
		n, _ := io.ReadFull(resp.Body, buf)
		resp.Body.Close()
		// The archive may start in the middle of a packet: look for the
		// sync byte repeating every 188 bytes.
		ts := -1
		for off := 0; off < 188 && off+188*3 < n; off++ {
			if buf[off] == 0x47 && buf[off+188] == 0x47 && buf[off+376] == 0x47 {
				ts = off
				break
			}
		}
		fmt.Printf("  %-40s HTTP %d, %s, %d byte, MPEG-TS %v (da +%d), %.1f s\n", label, resp.StatusCode,
			resp.Header.Get("Content-Type"), n, ts >= 0, max(ts, 0), time.Since(t0).Seconds())
		time.Sleep(3 * time.Second) // one connection: let the provider close it
	}
	get("diretta "+pick.Name.String(), c.StreamURL(xtream.KindLive, pick.StreamID.String(), "ts"))
	for _, p := range picks {
		get("archivio "+p.Name.String(), c.CatchupURL(p.StreamID.String(), start, 30*time.Minute, info.Location()))
	}
	get("archivio (vecchio formato .php) "+pick.Name.String(), c.CatchupPHPURL(pick.StreamID.String(), start, 30*time.Minute, info.Location()))
	rome, _ := time.LoadLocation("Europe/Rome")
	for _, z := range []struct {
		name string
		loc  *time.Location
	}{{"server", info.Location()}, {"UTC", time.UTC}, {"Europe/Rome", rome}} {
		u := c.CatchupURL(pick.StreamID.String(), start, 30*time.Minute, z.loc)
		t0 := time.Now()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "VodMax")
		resp, err := c.HTTP().Do(req)
		if err != nil {
			fmt.Printf("  %-12s errore: %v\n", z.name, provider.Redact(err))
			continue
		}
		buf := make([]byte, 188*20)
		n, _ := io.ReadFull(resp.Body, buf)
		resp.Body.Close()
		ts := n >= 188*2 && buf[0] == 0x47 && buf[188] == 0x47
		fmt.Printf("  %-12s HTTP %d, %d byte, MPEG-TS %v, %.1f s\n", z.name, resp.StatusCode, n, ts, time.Since(t0).Seconds())
		time.Sleep(2 * time.Second) // one connection: let the provider close it
	}
	return nil
}

func dump(c *xtream.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	names := map[string]string{}
	for kind, fetch := range map[string]func(context.Context) ([]xtream.Category, error){
		"live": c.LiveCategories, "movie": c.VODCategories, "series": c.SeriesCategories,
	} {
		cats, err := fetch(ctx)
		if err != nil {
			return err
		}
		for _, cat := range cats {
			names[kind+"/"+cat.ID.String()] = cat.Name.String()
		}
	}
	clean := func(s string) string { return strings.NewReplacer("\t", " ", "\n", " ").Replace(s) }
	live, err := c.LiveStreams(ctx)
	if err != nil {
		return err
	}
	for _, s := range live {
		fmt.Printf("live\t%s\t%s\t%s\n", clean(names["live/"+s.CategoryID.String()]), clean(s.Name.String()), s.EPGChannelID)
	}
	vod, err := c.VODStreams(ctx)
	if err != nil {
		return err
	}
	for _, s := range vod {
		fmt.Printf("movie\t%s\t%s\t%s\n", clean(names["movie/"+s.CategoryID.String()]), clean(s.Name.String()), s.Year)
	}
	series, err := c.Series(ctx)
	if err != nil {
		return err
	}
	for _, s := range series {
		fmt.Printf("series\t%s\t%s\t%s\n", clean(names["series/"+s.CategoryID.String()]), clean(s.Name.String()), s.ReleaseDate)
	}
	return nil
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
	fmt.Fprintln(os.Stderr, "uso: vodmax-probe vpn-check|vpn-up|xtream-login|xtream-dump|xtream-live-check|xtream-vod-check -conf wg.conf [-server URL -user NOME]")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "errore:", err)
	os.Exit(1)
}
