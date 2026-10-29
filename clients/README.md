# App VodMax

Le app usano l'interfaccia del server VodMax (la stessa del browser) e si
abbinano con il codice di Impostazioni → Dispositivi.

## Android (telefono e Android TV) — `android/`

Un solo APK. L'interfaccia è in una WebView; film, episodi, registrazioni
e catch-up sono riprodotti dal player nativo Media3 (MKV, AC-3/E-AC-3,
HEVC senza conversione sul server, tracce audio e sottotitoli). Il tasto
Indietro passa dalla pagina; il banner compare nel launcher di Android TV.

```sh
cd clients/android
echo "sdk.dir=/percorso/android-sdk" > local.properties
JAVA_HOME=/percorso/jdk-17 ./gradlew assembleDebug
# → app/build/outputs/apk/debug/app-debug.apk
```

Primo avvio: indirizzo del server (es. `http://192.168.1.10:8095`), poi il
codice di abbinamento. Server diverso: Impostazioni → Sistema → Cambia server.

## LG webOS — `webos/`

Un avvio leggero: chiede l'indirizzo del server, controlla che risponda e
apre l'interfaccia (player del browser della TV). Pacchetto con gli
strumenti LG:

```sh
npm install @webos-tools/cli
npx ares-package clients/webos -o dist      # → app.vodmax.client_<versione>_all.ipk
npx ares-setup-device                         # la TV in modalità sviluppatore
npx ares-install -d <tv> dist/app.vodmax.client_*.ipk
```

Compatibilità: interfaccia completa sulle TV LG dal 2022 (webOS 22); sulle
precedenti funziona ma l'impaginazione può essere imperfetta.
