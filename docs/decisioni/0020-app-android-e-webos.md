# 0020 — App Android e webOS: interfaccia del server, player nativo su Android

Stato: **Approvata** (29 ottobre 2026: "iniziamo a creare l'apk e l'app per webOS")

## Decisione

- **Un solo APK** per telefono e Android TV (`clients/android`, Java,
  Android 7+): l'interfaccia del server in una WebView, così menu, TV,
  guida e impostazioni restano una sola; film, episodi, registrazioni e
  catch-up passano al **player nativo Media3** (decisione 0018), che legge
  i file del provider così come sono (sessione "direct": niente
  conversione sul server) con tracce audio e sottotitoli. La posizione
  torna alla pagina e si salva come nel browser. TV in diretta per ora
  nella pagina (hls.js/mpegts.js funzionano nella WebView).
- **webOS** (`clients/webos`): avvio leggero che ricorda l'indirizzo del
  server, controlla che risponda e apre l'interfaccia. Player del
  browser della TV.
- Abbinamento con il codice esistente (decisione 0019). L'indirizzo del
  server si inserisce al primo avvio: nessun indirizzo nel codice.
- Firma: per ora la chiave di debug; prima di distribuire l'APK a più
  persone serve una chiave di rilascio conservata in modo sicuro (gli
  aggiornamenti devono essere firmati sempre con la stessa).

## Da fare

- TV in diretta con il player nativo su Android TV (zapping P+/P− dentro
  il player), ricerca dei server in rete locale, chiave di rilascio,
  compilazione degli APK nella CI, impaginazione per webOS prima del 2022.
