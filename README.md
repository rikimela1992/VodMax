# VodMax

Media center in stile Plex / Jellyfin / IPTV Extreme: aggiungi i tuoi
**abbonamenti** (playlist M3U o accessi Xtream Codes, con guida TV) e ritrovi
tutto in una sola Home, una sola ricerca, una sola lista. Ogni abbonamento
può passare da una **VPN** registrata.

Un'unica applicazione che funziona da sola su un dispositivo (**Agent**) e
che può diventare il **Server** della casa per gli altri dispositivi.

> Stato: **sviluppo iniziale**. Primo modulo: VPN interna, client verso il
> provider che passa solo dal tunnel, lettura di playlist M3U e Xtream Codes.

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
