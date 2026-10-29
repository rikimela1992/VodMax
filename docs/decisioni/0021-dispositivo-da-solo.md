# 0021 — Dispositivo da solo: il server dentro l'app Android

Stato: **Approvata** (29 ottobre 2026: "Si, procedi", per installare
VodMax su una Firestick senza server dietro)

## Decisione

- L'APK contiene il **server VodMax stesso** (lo stesso programma Go, per
  arm64, armv7 e x86_64), installato come `libvodmax.so` perché Android
  permetta di eseguirlo. È l'idea "Agent" della decisione 0001, fatta
  senza riscrivere nulla: interfaccia, catalogo, guida, profili e VPN
  interna sono quelli del server.
- Al primo avvio si sceglie **"Collegati a un server"** (come prima) o
  **"Usa questo dispositivo da solo"**: l'app avvia il server su
  `127.0.0.1:8095` (solo loopback), entra con il suo token di
  amministrazione e, senza abbonamenti, apre Impostazioni → Abbonamenti.
- Modalità `-device` del server: **niente registrazioni e niente pausa
  della diretta** (sul dispositivo non c'è ffmpeg, e lo spazio è poco).
  Film ed episodi vanno al player nativo così come sono; la diretta
  passa dal relay come senza timeshift.
- L'app riavvia il server se Android lo ha fermato in background; la
  sincronizzazione notturna recupera all'avvio successivo.

## Limiti e attenzioni

- Fire TV: solo i modelli con Fire OS (Android). La Fire TV Stick 4K
  Select (2025) usa Vega OS e non installa APK.
- Il provider è contattato **dal dispositivo** (dalla VPN interna, o
  direttamente con la VPN "di sistema"): vale la regola di rete di chi lo
  usa. Un abbonamento con una sola connessione è condiviso con il server
  di casa.
- APK più pesante (~28 MB con tre architetture).

## Da fare

- Catch-up (oggi convertito da ffmpeg), tasti del telecomando Fire TV
  (niente P+/P− né tasti colorati), APK separati per architettura.
