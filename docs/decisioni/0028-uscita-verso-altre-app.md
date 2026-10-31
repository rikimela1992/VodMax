# 0028 — Uscita verso altre app (HDHomeRun, M3U, XMLTV)

Stato: **Approvata** (31 ottobre 2026: "inizia a fare tutti i punti che hai
elencato prima")

## Decisione

- VodMax può dare i suoi canali ad altre app di casa, spento di default
  (Impostazioni → Sistema → Uscita verso altre app):
  - **sintonizzatore HDHomeRun emulato** (`discover.json`, `lineup.json`,
    `lineup_status.json`): Plex, Jellyfin ed Emby lo aggiungono e registrano
    con il loro DVR;
  - **playlist M3U** (`tvg-id`, `tvg-chno`, logo, gruppo) e **guida XMLTV**
    (da un giorno fa alla fine della guida, con i segni live/replica).
- Gli indirizzi sono `/out/<chiave>/…`: lettori e sintonizzatori non sanno
  autenticarsi, quindi la chiave segreta (32 caratteri, confronto a tempo
  costante) è il permesso. "Cambia la chiave" blocca i vecchi indirizzi.
  Spento o chiave sbagliata: 404.
- I flussi passano da VodMax come i suoi: stesso limite di connessioni,
  stessa VPN, registrazione in corso riusata. Ogni richiesta è uno
  spettatore a sé. Sintonizzatori dichiarati: le connessioni degli
  abbonamenti (1–8).
- Canali: quelli visibili, nell'ordine del telecomando, senza contenuti per
  adulti (come il profilo principale).

## Non ancora

- API Xtream in uscita (per le app che vogliono anche film e serie).
