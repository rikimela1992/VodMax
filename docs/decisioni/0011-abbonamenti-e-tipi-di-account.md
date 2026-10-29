# 0011 — Abbonamenti: solo playlist

Stato: **Approvata** (29 ottobre 2026) · Estende la [0002](0002-sorgenti.md)

## Decisione

Un **abbonamento** è una **playlist** di un provider. VodMax ne unisce i
contenuti in una sola Home, una sola ricerca, una sola lista, come IPTV
Extreme / TiviMate.

| Tipo di playlist | Come si aggiunge | Contenuti |
|---|---|---|
| **M3U / M3U8** | URL oppure file | Diretta e VOD; `tvg-id`, `tvg-logo`, `group-title` |
| **Xtream Codes** | Server, utente, password | Diretta, film, serie, catch-up, EPG |
| **EPG XMLTV** (accessoria) | URL oppure file, legata a una playlist | Guida TV |

Ogni abbonamento può avere una **connessione VPN** associata (vedi
[0012](0012-connessioni-vpn.md)).

## Scartato

- **Servizi di streaming** (Netflix, Prime Video, Disney+, Paramount+, …):
  non riproducibili per via del DRM. Anche la sola vista del catalogo con
  "Apri nell'app" è stata scartata dall'utente: fuori dal progetto.

## Da rivalutare più avanti

Fuori dalla prima fase, possono tornare come nuovi tipi di abbonamento:
file locali sul Server, librerie Jellyfin / Plex, Tvheadend.

## Modello dati

```
Abbonamento
  id, nome scelto dall'utente
  tipo: m3u | xtream
  m3u:    url o file, intestazioni HTTP facoltative (User-Agent)
  xtream: url server, utente, password (cifrata a riposo)
  epg: zero o più URL/file XMLTV, abbinamento canali per tvg-id e nome
  vpn: connessione associata (facoltativa)
  aggiornamento automatico: intervallo, ultima sincronizzazione, esito
  stato account (Xtream): attivo / scaduto, scadenza, connessioni massime
```

L'id di ogni canale o titolo nasce da **abbonamento + id nella playlist**
(`stream_id` Xtream, oppure `tvg-id`/URL normalizzato per M3U), mai dal
titolo.

## Domande aperte

- Stesso film presente in due abbonamenti: un'unica scheda con più versioni
  (proposta: sì, come nel VodMax precedente)?
- Serve il supporto Stalker / MAC portal?
