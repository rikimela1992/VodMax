# 0022 — Raccolte locali: cartelle di film e serie

Stato: **Approvata** (30 ottobre 2026: "chiunque lo usa deve poter
scegliere una cartella sorgente")

## Decisione

- Una **raccolta** è una cartella del server (o una condivisione montata
  sul server) che contiene **film** oppure **serie** (una sottocartella per
  serie). Si aggiunge da Impostazioni → Raccolte, scrivendo il percorso o
  sfogliando le cartelle del server (anche con il telecomando).
- Internamente è un abbonamento di tipo `local`, senza VPN: catalogo,
  ricerca, schede, varianti, "Continua a guardare", profili e player sono
  gli stessi del provider. I file non passano dalla VPN e non hanno limite
  di connessioni.
- **Titoli dai nomi**, come li scrivono Jellyfin, Kodi e Plex:
  `Il Padrino (1972).mkv`, `Il Padrino (1972)/file.mkv`,
  `The.Godfather.1972.1080p.BluRay.mkv`; episodi da `S01E03`, `1x03` o
  `Stagione 1/Episodio 3`. **File `.nfo`** (titolo, anno, trama, cast,
  regia, voto, id TMDB) e **immagini** accanto ai video (`poster`,
  `folder`, `fanart`) quando ci sono; altrimenti TMDB, se c'è la chiave.
- Lo **stesso film** in raccolta e dal provider è **una sola scheda**: la
  copia della raccolta è la prima scelta.
- Lettura: all'aggiunta, ogni notte con il catalogo e con "Aggiorna ora".
  Una cartella sparita o svuotata a metà (disco non montato) non cancella
  il catalogo: vale la stessa protezione degli abbonamenti.
- Permessi e montaggi sono di chi installa: VodMax legge ciò che l'utente
  del servizio può leggere, e non scrive mai nelle raccolte.

## Da fare

- Raccolte sul dispositivo nell'app Android "da sola" (permessi di
  Android sui file), sottotitoli `.srt` accanto ai video, musica e foto.
