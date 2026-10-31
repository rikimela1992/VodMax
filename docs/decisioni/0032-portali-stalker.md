# 0032 — Portali Stalker (MAC) e diretta M3U dal server

Stato: **Approvata** (31 ottobre 2026: "inizia a fare tutti i punti che hai
elencato prima")

## Decisione

- Nuovo tipo di abbonamento **portale Stalker / Ministra**: indirizzo del
  portale (come lo si trova: `…/c/`, `…/stalker_portal/c/` o l'indirizzo
  dell'API) e indirizzo MAC del box. Il MAC è la credenziale: resta cifrato
  come le password e non viene mai mostrato.
- VodMax si presenta come un MAG250: handshake, `get_profile`, generi e
  canali. Il comando di ogni canale resta cifrato nel catalogo; a ogni
  visione o registrazione si chiede al portale un link nuovo
  (`create_link`). Se il portale ha dimenticato la sessione, VodMax rientra
  da solo una volta.
- Per ora **solo canali in diretta**, con una connessione alla volta (un box
  guarda un canale). La guida si aggiunge da una fonte XMLTV: i canali
  portano il loro `xmltv_id`.
- Tutto passa dalla VPN dell'abbonamento, come per Xtream.
- Con lo stesso cambiamento l'indirizzo della diretta si sceglie per tipo di
  abbonamento: Xtream (costruito), M3U (quello della playlist, finora non
  usato dal server), Stalker (chiesto al portale). Quindi anche i canali
  delle playlist M3U ora si guardano e si registrano tramite il server.

## Non ancora

- Film e serie dei portali, guida e replay del portale.
- Verifica su un portale vero: provato con un portale finto nei test.
