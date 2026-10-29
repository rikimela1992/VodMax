# 0010 — Perimetro della prima versione

Stato: **Proposta**

## Obiettivo della 0.1

Un **Agent Android autonomo** che sostituisce un player IPTV classico:

- aggiungere una sorgente Xtream Codes o M3U + EPG XMLTV;
- Diretta con guida TV, Film, Serie con stagioni ed episodi;
- ricerca istantanea, preferiti, "Continua a guardare", episodio successivo;
- un profilo, tema chiaro/scuro;
- interfaccia telefono e TV.

## Subito dopo (0.2)

- Interruttore **Agent → Server** con abbinamento di un secondo dispositivo.
- Interfaccia web servita dall'Agent promosso.
- Più profili.

## Poi (0.3+)

- Server Docker, registrazioni, arricchimento TMDB, app webOS.

## Fuori dal perimetro iniziale

Transcodifica, accesso da internet, iOS, cartelle locali, import da Jellyfin.

## Criteri di "fatto" per ogni versione

- Test automatici verdi, compresi quelli di velocità su catalogo sintetico
  da 200.000 elementi.
- Nessun dato reale, segreto o riferimento alla rete di casa nel repository.
