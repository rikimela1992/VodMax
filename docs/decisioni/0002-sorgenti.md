# 0002 — Sorgenti supportate

Stato: **Proposta** · Il perimetro delle sorgenti è fissato dalla
[0011](0011-abbonamenti-e-tipi-di-account.md): solo playlist M3U e Xtream
Codes con EPG. Le sorgenti "successive" qui sotto sono da rivalutare.

## Proposta

Ogni sorgente è un **adattatore** con la stessa interfaccia: elenca canali,
film, serie ed episodi; fornisce l'URL di riproduzione; dichiara cosa sa fare
(diretta, VOD, catch-up, EPG).

Prima versione:

| Sorgente | Contenuti | Note |
|---|---|---|
| **Xtream Codes API** | Diretta, film, serie, EPG, catch-up | Lo standard di fatto dei provider IPTV. Metadati già strutturati. |
| **Playlist M3U / M3U8** | Diretta, VOD | Con `tvg-id`, `tvg-logo`, `group-title`. |
| **EPG XMLTV** | Guida TV | Da URL o file; abbinamento canali per `tvg-id` e nome. |

Successive, se servono:

- **Cartella locale / condivisione di rete** (film e serie propri, con NFO).
- **Tvheadend** (diretta e registrazioni da sintonizzatore).
- **Jellyfin / Emby** in sola lettura, per importare una libreria esistente.

## Metadati e copertine

- Prima quelli forniti dalla sorgente (Xtream li ha già).
- Arricchimento opzionale da **TMDB** con chiave dell'utente, cercando
  titolo **senza anno né tag di qualità** e anno separato. È la lezione del
  VodMax precedente: "1408 (2007)" su TMDB non trova nulla, "1408" + 2007 sì.
- Copertine scaricate una volta, ridimensionate (WebP) e messe in cache con
  un limite di spazio.

## Identità degli elementi

L'id interno nasce da **sorgente + id della sorgente** (per esempio lo
`stream_id` Xtream), mai dal percorso di un file o dal titolo. Così un titolo
che cambia nome o cartella non perde preferiti, progressi e copertina.

## Domande aperte

- Più sorgenti con lo stesso film: unirle in un'unica scheda con più
  versioni (qualità, lingua)? Il VodMax precedente lo faceva ed era utile.
- Serve il supporto Stalker / MAC portal?
