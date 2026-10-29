# 0011 — Abbonamenti e tipi di account

Stato: **Proposta** · Estende la [0002](0002-sorgenti.md)

## L'idea

L'utente aggiunge i propri **abbonamenti**: ognuno è un account presso un
provider, di un certo **tipo**. VodMax ne unisce i cataloghi in una sola Home,
una sola ricerca, una sola lista. È il modello Plex / Jellyfin / IPTV Extreme
esteso ai servizi di streaming.

## Due famiglie di abbonamento

### A. Riproducibili dentro VodMax

| Tipo | Dove | Note |
|---|---|---|
| **Xtream Codes** | Agent e Server | Diretta, film, serie, EPG, catch-up |
| **Playlist M3U / M3U8** (+ EPG XMLTV) | Agent e Server | Da URL o file |
| **File locali** | Solo Server | Cartelle di film e serie sul disco o sul NAS |
| Jellyfin / Plex / Emby (in seguito) | Agent e Server | Importa una libreria esistente |
| Tvheadend (in seguito) | Server | Sintonizzatore e registrazioni |

VodMax riproduce con il proprio player, salva i progressi, gestisce
"Continua a guardare", registrazioni e passaggio dal Server.

### B. Servizi di streaming: catalogo e "Apri nell'app"

| Tipo | Esempi |
|---|---|
| **Servizio in abbonamento** | Netflix, Prime Video, Disney+, Paramount+, NOW, Apple TV+ |
| **Servizio gratuito** | RaiPlay, Mediaset Infinity, Pluto TV |

Cosa fa VodMax:

- sa **quali titoli sono disponibili** su ogni servizio che l'utente dichiara
  di avere, nel suo paese, tramite i dati "dove guardarlo" di TMDB
  (`watch/providers`, forniti da JustWatch, con attribuzione obbligatoria);
- li mostra nella Home, nella ricerca e nella lista insieme ai contenuti di
  tipo A, con il marchio del servizio ("Su Netflix");
- **Guarda** apre l'app ufficiale direttamente sul titolo (link diretto /
  intent Android; su webOS avvio dell'app del servizio), dove l'utente è già
  autenticato;
- se lo stesso film è sia su un abbonamento A sia su un servizio B, mostra
  entrambe le opzioni.

Cosa **non** fa, e perché:

- **Non riproduce questi contenuti nel proprio player.** Sono protetti da DRM
  (Widevine / PlayReady): le licenze di decodifica sono rilasciate solo alle
  app autorizzate. Aggirarle viola le condizioni d'uso e la normativa sul
  diritto d'autore (protezione delle misure tecnologiche).
- **Non chiede né memorizza le password** di Netflix e simili: non servono,
  perché il login resta nell'app ufficiale.
- **Non registra né scarica** contenuti di questi servizi.
- **Non conosce i progressi** fatti nell'app ufficiale (nessuna API pubblica):
  "Continua a guardare" vale solo per i contenuti di tipo A.

## Modello dati

```
Abbonamento
  id, nome scelto dall'utente, tipo, famiglia (A | B)
  configurazione del tipo (URL, utente, password cifrata, cartelle…) — solo A
  paese / regione — solo B
  connessione VPN associata (facoltativa, vedi 0012)
  stato: attivo, scaduto, errore, ultima sincronizzazione
```

Un tipo è un **adattatore** (vedi 0002) che dichiara le proprie capacità:
`diretta`, `vod`, `serie`, `epg`, `catch-up`, `riproduzione-interna`,
`apri-in-app`. L'interfaccia mostra solo ciò che il tipo sa fare.

## Domande aperte

- Quali servizi B servono davvero all'inizio? Proposta: Netflix, Prime Video,
  Disney+, Paramount+, RaiPlay.
- Mostrare i titoli B anche a chi non ha quell'abbonamento ("Disponibile su
  Disney+, non hai l'abbonamento")? Proposta: no, solo i servizi dichiarati.
- La chiave TMDB la inserisce l'utente o ne usa una del progetto?
