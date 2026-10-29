# VodMax

Media center in stile Plex / Jellyfin / IPTV Extreme: aggiungi i tuoi
**abbonamenti** (playlist M3U o accessi Xtream Codes, con guida TV) e ritrovi
tutto in una sola Home, una sola ricerca, una sola lista. Ogni abbonamento
può passare da una **VPN** registrata.

Un'unica applicazione che funziona da sola su un dispositivo (**Agent**) e
che può diventare il **Server** della casa per gli altri dispositivi.

> Stato: **sviluppo iniziale**, provato su un catalogo reale (non ancora su
> una TV). Funzionano:
>
> - VPN interna (che si ricollega da sola); abbonamenti Xtream e M3U;
>   sincronizzazione solo tramite VPN, ogni notte e a richiesta;
> - **selezione del catalogo** (paesi, adulti, anno minimo, categorie, una
>   scheda per titolo) con anteprima;
> - **TV** come un decoder (numerazione da telecomando, preferiti, comandi a
>   schermo), **guida a griglia** anche da fonti XMLTV esterne;
> - **film e serie** con "Continua a guardare", lingua dell'audio e
>   sottotitoli, schede dal provider e (con una chiave) da TMDB;
> - **registrazioni** dal programma in onda, dalla guida o **per regola**,
>   con controllo delle connessioni e dello spazio;
> - **profili** (anche bambini) e **dispositivi abbinati** con un codice;
> - **backup** cifrati e copie automatiche; avvisi su VPN, abbonamenti,
>   guida e disco.
>
> Pagine: menu `/`, TV `/tv.html`, guida `/guide.html`, impostazioni
> `/settings.html`.

## L'idea in breve

| Modalità | Dove gira | Cosa fa |
|---|---|---|
| **Agent** | Android (telefono, TV box, Android TV) | Player autonomo come IPTV Extreme / TiviMate: aggiungi una sorgente (M3U, Xtream Codes, EPG XMLTV) e guardi. Nessun server necessario. |
| **Agent → Server** | Lo stesso Android, acceso in casa | Con un interruttore l'Agent espone catalogo, profili, progressi ed EPG agli altri dispositivi della rete. |
| **Server** | Linux / Docker / NAS | Stessa logica, senza interfaccia locale: sempre acceso, registrazioni, più utenti, interfaccia web. |

Un dispositivo che trova un Server in rete può collegarsi a lui invece di
gestire le sorgenti da solo: profili, preferiti e "Continua a guardare"
diventano condivisi.

## Abbonamenti

Solo **playlist**: M3U / M3U8 (URL o file) e Xtream Codes, con guida TV
XMLTV. Dettagli nella
[decisione 0011](docs/decisioni/0011-abbonamenti-e-tipi-di-account.md).

**Nessun contatto col provider senza VPN.** Server e app hanno una VPN
WireGuard interna: se non è collegata, gli abbonamenti restano bloccati.
Dettagli nella [decisione 0012](docs/decisioni/0012-connessioni-vpn.md).

## Principi

- **Nessun contenuto incluso.** VodMax è un player: le sorgenti le aggiunge
  l'utente. Nessun provider, URL o credenziale nel codice.
- **Veloce con cataloghi enormi.** Obiettivo: Home e liste sotto i 300 ms con
  oltre 200.000 elementi (il caso che oggi rende lento Jellyfin).
- **Funziona offline dal server.** L'Agent non dipende mai da un Server per
  guardare le proprie sorgenti.
- **Un solo nucleo.** Agent e Server condividono la stessa logica; cambia solo
  chi la ospita.

## Documentazione

- [Decisioni di base](docs/decisioni/README.md): una scheda per ogni scelta,
  con opzioni, raccomandazione e domande aperte.
- [Spunti dal VodMax precedente](docs/spunti-vodmax-precedente.md): cosa
  riprendere (grafica e soluzioni) e cosa evitare. Nessun codice riutilizzato.

## Struttura del codice

| Pacchetto | Cosa fa |
|---|---|
| `internal/vpn` | Lettura dei file WireGuard `.conf`; tunnel WireGuard in user space con stack di rete proprio. DNS compreso, tutto resta nel tunnel. |
| `internal/provider` | L'unico client HTTP ammesso verso i provider: si collega solo tramite il tunnel, ignora i proxy di sistema, toglie le credenziali dagli errori. |
| `internal/playlist/m3u` | Lettura di playlist M3U estese (`tvg-*`, `group-title`, catch-up, `#EXTVLCOPT`). |
| `internal/playlist/xtream` | Client Xtream Codes: account, diretta, film, serie, episodi, URL di riproduzione e catch-up. |
| `internal/secret` | Cifratura AES-256-GCM delle credenziali con chiave locale (`secret.key`, 0600). |
| `internal/store` | SQLite: lettori e scrittore separati (WAL), id stabili, import "tutto o niente" con protezione dai cali anomali, ricerca FTS5 senza accenti, paginazione a cursore. |
| `internal/catalog` | Conversione delle playlist in catalogo; titoli puliti (anno separato, tag di qualità tolti), separatori scartati. |
| `internal/service`, `internal/api` | Logica e API HTTP `/api/v1` (VPN, abbonamenti, sincronizzazione, catalogo, categorie), protetta da token. |
| `internal/lineup` | Riconoscimento dei canali nazionali e numerazione LCN ufficiale. |
| `internal/epg` | Lettura in streaming delle guide XMLTV. |
| `internal/web` | Interfaccia da telecomando: menu (`/`: Home, Film, Serie, Registrazioni), TV (`/tv.html`), Impostazioni (`/settings.html`: abbonamenti, VPN, selezione del catalogo con anteprima, categorie); HTML/JS senza framework, mpegts.js 1.8.2 incluso. Un link `/#token=…` salva il token di accesso. |
| `cmd/vodmax-server` | Il server. |
| `cmd/vodmax-probe` | Strumento di prova: `.conf`, tunnel, login Xtream. |

## Avvio del server

```sh
go build ./cmd/vodmax-server
./vodmax-server -data ./vodmax-data -listen 127.0.0.1:8095
```

Al primo avvio crea in `-data` il database, la chiave di cifratura
(`secret.key`) e il token di amministrazione (`admin.token`). Ogni chiamata
API, tranne `/api/v1/health`, richiede `Authorization: Bearer <token>`.

| Metodo e percorso | Scopo |
|---|---|
| `POST /api/v1/vpn` `{name, config}` | Registra una VPN WireGuard (testo `.conf`) |
| `POST /api/v1/vpn/{id}/connect` · `/disconnect` | Collega o scollega il tunnel |
| `POST /api/v1/subscriptions` `{name, kind, vpn_id, server, username, password}` o `{…, url}` | Aggiunge un abbonamento Xtream o M3U |
| `POST /api/v1/subscriptions/{id}/sync` | Sincronizza il catalogo (`?allow_drop=true` per accettare un calo anomalo) |
| `GET /api/v1/catalog/{live\|movie\|series}` | Catalogo selezionato, una scheda per titolo: `q`, `section` (tv, sport, radio), `category`, `subscription`, `sort=recent`, `cursor`, `limit`, `all=true` per vedere anche l'escluso |
| `GET /api/v1/categories/{kind}` | Categorie |
| `GET` · `PUT /api/v1/subscriptions/{id}/rules` | Regole di selezione (paesi, adulti, SD, anno minimo) |
| `POST /api/v1/subscriptions/{id}/rules/preview` | Effetto di nuove regole senza salvarle |
| `GET /api/v1/subscriptions/{id}/selection` | Conteggi: offerti, visibili, schede, per sezione |
| `GET /api/v1/subscriptions/{id}/categories?kind=` | Categorie con classificazione, correzioni e motivo dell'esclusione |
| `PUT /api/v1/subscriptions/{id}/categories/{kind}/{categoria}` | Correzione manuale: `enabled`, `section`, `country`, `adult` (`null` = automatico) |
| `PUT /api/v1/items/{id}/hidden` | Nasconde o ripristina un singolo titolo o canale |
| `GET /api/v1/items/{id}/variants` | Le versioni dello stesso titolo, migliore per prima |
| `GET /api/v1/tv/channels` | Canali in ordine da telecomando con numero, ora e a seguire: `section`, `list=favorites` |
| `GET /api/v1/tv/channels/{id}/guide` | Guida di un canale (`hours`, massimo 72) |
| `PUT /api/v1/tv/favorites/{id}` `{favorite}` | Aggiunge o toglie un canale dai preferiti |
| `POST /api/v1/subscriptions/{id}/guide/sync` | Aggiorna la guida (fatto anche a ogni sincronizzazione) |
| `POST /api/v1/live/{id}/ticket` `{viewer}` | Permesso di visione monouso (30 s) → `{url}` |
| `GET /stream/live/{permesso}` | Il flusso MPEG-TS ritrasmesso dal server (`?force=1` per prendere il controllo della connessione) |
| `GET /api/v1/series/{id}/episodes` | Episodi di una serie, con avanzamento |
| `GET` · `PUT /api/v1/progress/{movie\|episode}/{id}` | Avanzamento (`position`, `duration` in secondi) |
| `GET /api/v1/continue` | Continua a guardare |
| `POST /api/v1/vod/{movie\|episode}/{id}/session` `{viewer, mode?}` | Sessione di riproduzione: `direct` (MP4, intervalli) o `remux` (ffmpeg, `?t=` secondi) |
| `GET` · `POST /api/v1/recordings` | Registrazioni; `{channel_id}` registra il programma in onda, oppure `start`/`stop` esatti |
| `DELETE /api/v1/recordings/{id}` · `POST …/{id}/ticket` | Annulla o elimina; permesso di visione |
| `GET /img/logo/{id}?s=firma` | Logo del canale scaricato tramite VPN e in cache (indirizzo firmato, fornito dall'elenco dei canali) |

## Sviluppo

Serve Go (versione indicata in `go.mod`).

```sh
go vet ./...
go test -race ./...
```

I test del tunnel creano due VPN interne nello stesso processo e verificano
che il traffico passi dal tunnel, che a VPN chiusa non parta nessuna
connessione e che un server VPN irraggiungibile non causi fughe di traffico.

## Licenza

Da decidere prima della pubblicazione (vedi
[decisione 0009](docs/decisioni/0009-licenza-e-pubblicazione.md)). Finché il
repository è privato, tutti i diritti sono riservati.
