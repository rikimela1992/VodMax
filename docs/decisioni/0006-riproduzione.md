# 0006 — Riproduzione

Stato: **Proposta**

## Proposta

- **Diretta dalla sorgente per default.** Il dispositivo apre direttamente
  l'URL del provider: nessun carico sul Server, nessuna transcodifica.
- **Passaggio dal Server solo quando serve**, scelto per sorgente:
  - il provider accetta poche connessioni contemporanee e il Server deve
    gestirle (coda, "chi sta guardando cosa");
  - la rete del provider è raggiungibile solo dal Server (VPN, VLAN dedicata);
  - il formato non è supportato dal client (per esempio HEVC su un browser),
    con conversione opzionale tramite ffmpeg, **solo sul Server**.
- **Player**: Media3 / ExoPlayer su Android; HLS nativo o hls.js sul web.
- Ripresa dal punto lasciato, salvata ogni 15 secondi e alla chiusura.
  "Finito" a partire dal 95% o dai titoli di coda; un titolo finito esce da
  "Continua a guardare".
- Episodio successivo automatico con conto alla rovescia.

## Domande aperte

- Serve limitare il numero di visioni contemporanee per sorgente?
- Il Server deve poter convertire in tempo reale (richiede CPU), o basta il
  passaggio senza conversione?
