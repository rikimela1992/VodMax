# 0023 — Lingue dell'interfaccia

Stato: **Approvata** (30 ottobre 2026: "aggiungiamo la selezione della lingua")

## Decisione

- **Italiano e inglese**. I testi restano scritti in italiano nel codice e
  passano da `t("…")`; ogni lingua è un dizionario (`internal/web/static/i18n/
  <lingua>-*.js`) con il testo italiano come chiave. Un testo senza
  traduzione resta in italiano.
- La lingua è **per dispositivo**: quella del dispositivo al primo avvio
  (italiano se è italiano, altrimenti inglese), poi la scelta fatta in
  Impostazioni → Lingua o nella schermata "Chi guarda?" (anche per i profili
  non amministratori).
- Il server riceve la lingua (`X-VodMax-Lang`, altrimenti
  `Accept-Language`) e risponde nella stessa lingua: errori e avvisi sono
  formati in italiano e tradotti dal catalogo `messages_<lingua>.go`; un
  test verifica che ogni messaggio abbia la traduzione con gli stessi
  segnaposto.
- App Android (`values-en`) e avvio webOS seguono la lingua del sistema.

## Limiti

- Restano nella lingua in cui sono stati salvati gli errori memorizzati
  (ultima sincronizzazione, registrazioni fallite) e i testi dei provider
  (titoli, guida, categorie). Trame da TMDB in italiano.
- Nuova lingua: un dizionario per area, `messages_<lingua>.go`, e la voce in
  `LANGS` (`i18n.js`) e `Languages` (`lang.go`).
