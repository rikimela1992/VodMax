# Installare VodMax

[English](install.md)

VodMax ha due parti: il **server**, che tiene catalogo, guida,
registrazioni e avanzamenti di tutti, e le **app** (browser, Android,
Android TV, Fire TV, LG webOS) che lo mostrano. Un dispositivo Android può
anche usare VodMax **da solo**, senza server (vedi [Fire TV e Android senza
server](#fire-tv-e-android-senza-server)).

I pacchetti sono nelle [release](https://github.com/rikimela1992/VodMax/releases):

| File | Per |
|---|---|
| `vodmax-server_<v>_linux_amd64.tar.gz` | Server su PC, mini PC, macchina virtuale o container (x86-64) |
| `vodmax-server_<v>_linux_arm64.tar.gz` | Server su Raspberry Pi 4/5 o NAS ARM (64 bit) |
| `vodmax-server_<v>_linux_armv7.tar.gz` | Server su schede ARM a 32 bit |
| `VodMax-<v>.apk` | Telefoni e tablet Android, Android TV, Google TV, Fire TV |
| `app.vodmax.client_<v>_all.ipk` | TV LG con webOS |
| `SHA256SUMS` | Checksum: `sha256sum -c SHA256SUMS` |

## 1. Server

Una qualsiasi macchina Linux sempre accesa: mini PC, NAS, Raspberry Pi,
macchina virtuale o container LXC. Serve poco: qualche centinaio di MB di
memoria e circa 200 MB di disco per un catalogo di oltre 100.000 titoli, più
lo spazio per le registrazioni.

```sh
tar xzf vodmax-server_*_linux_amd64.tar.gz
cd vodmax-server_*/
sudo useradd --system --home /var/lib/vodmax --shell /usr/sbin/nologin vodmax
sudo install -m 755 vodmax-server /usr/local/bin/
sudo install -d -o vodmax -g vodmax -m 700 /var/lib/vodmax
sudo cp vodmax.service /etc/systemd/system/
sudo systemctl enable --now vodmax
vodmax-server -version
```

- **ffmpeg**: installalo dalla distribuzione (`sudo apt install ffmpeg`).
  Senza, VodMax funziona, ma niente pausa della diretta, niente
  registrazioni, e i file MKV/AVI non partono nel browser (l'app Android li
  riproduce comunque).
- **Porta**: l'interfaccia è su `http://<server>:8095`; si cambia in
  `vodmax.service` (`-listen`). Tienila dentro la rete di casa; da fuori usa
  una tua VPN (WireGuard, Tailscale…) invece di aprire la porta.
- **Dati**: tutto in `/var/lib/vodmax` (database, chiave di cifratura,
  token di amministrazione, registrazioni, backup).
- **Cartelle delle raccolte**: il servizio gira come utente `vodmax` e, con
  il file fornito, non vede `/home` né `/tmp`. Metti o monta le cartelle dei
  film sotto `/mnt`, `/srv` o `/media`, leggibili dall'utente `vodmax`.

### Aggiornare il server

```sh
sudo install -m 755 vodmax-server /usr/local/bin/vodmax-server
sudo systemctl restart vodmax
```

Il database si aggiorna da solo all'avvio. Prima fai un backup
(Impostazioni → Sistema → Backup).

## 2. Prima configurazione

1. Apri `http://<server>:8095` nel browser. Chiede il **token di
   amministrazione**: lo trovi sul server con
   `sudo cat /var/lib/vodmax/admin.token`.
2. **Impostazioni → Lingua**: italiano o inglese (per dispositivo; la stessa
   scelta è nella schermata dei profili).
3. **Impostazioni → VPN**: VodMax contatta le playlist **solo tramite una
   VPN**. Importa il file WireGuard `.conf` del tuo servizio VPN (Proton
   VPN, Mullvad, NordVPN…; meglio un file per ogni dispositivo), oppure
   indica una *VPN esterna* se il server esce già da una VPN sul router (in
   quel caso sta a te che il traffico passi davvero da lì).
4. **Impostazioni → Abbonamenti**: aggiungi una playlist M3U (il suo
   indirizzo) o un account Xtream Codes (server, utente, password), scegli
   la VPN, poi *Sincronizza ora*. Catalogo e guida si aggiornano ogni notte,
   la guida anche ogni poche ore (Impostazioni → Sistema).
5. **Impostazioni → Selezione** e **Categorie**: tieni solo i paesi, gli
   anni e le categorie che vuoi; l'anteprima mostra l'effetto prima di
   salvare.
6. **Impostazioni → Raccolte** (facoltativo): aggiungi cartelle di film o di
   serie (una sottocartella per serie). Nomi come `Film (2010).mkv` e
   `Serie/Stagione 1/S01E03.mkv` vengono riconosciuti; i file `.nfo` e le
   immagini accanto ai video si usano quando ci sono.
7. **Impostazioni → Profili** (facoltativo): un profilo per persona, profili
   bambini, PIN.
8. **Impostazioni → Sistema → Copertine e schede (TMDB)** (facoltativo): con
   una chiave TMDB gratuita arrivano locandine, trame e cast dove la
   playlist non li ha.

## 3. Dispositivi

Ogni dispositivo si **abbina** una volta: mostra un codice, che approvi in
**Impostazioni → Dispositivi** da un dispositivo già collegato. Il codice
vale 10 minuti.

### Browser (PC, Mac, tablet, qualsiasi TV)

Apri `http://<server>:8095`. Funziona in ogni browser recente, anche in
quello delle TV Samsung, Philips e delle altre smart TV (senza i tasti
colorati del telecomando).

### Telefono e tablet Android, Android TV, Google TV

1. Scarica `VodMax-<v>.apk` sul dispositivo (o copialo con una chiavetta
   USB).
2. Quando Android lo chiede, permetti l'installazione da quella fonte
   (*Installa app sconosciute*).
3. Apri VodMax, scrivi l'indirizzo del server (`http://192.168.1.10:8095`),
   poi approva il codice di abbinamento.

Film e serie partono nel player nativo (ogni formato, tracce audio e
sottotitoli); il resto è la stessa interfaccia del browser. Su Android TV
l'app compare nel launcher con il suo banner.

### Fire TV

Funziona sui Fire TV con **Fire OS** (Fire TV Stick Lite, HD, 4K, 4K Max,
Fire TV Cube, TV Fire TV). **Non** funziona sulla Fire TV Stick 4K Select
(2025), che usa Vega OS e non installa app esterne allo store.

1. *Impostazioni → La mia Fire TV → Informazioni*: clicca 7 volte sul nome
   del dispositivo per far comparire *Opzioni sviluppatore*; lì attiva *App
   di origine sconosciuta* (o permettilo all'app Downloader).
2. Installa **Downloader** dall'Amazon Appstore e aprici il link di
   `VodMax-<v>.apk` dalla pagina della release.
3. Apri VodMax: scrivi l'indirizzo del server, oppure scegli *Usa questo
   dispositivo da solo*.

Il telecomando Fire TV non ha tasti dei canali né colorati: in diretta usa
Su/Giù e OK (elenco dei canali).

### Fire TV e Android senza server

Nella prima schermata scegli **Usa questo dispositivo da solo**: l'app
avvia un server VodMax tutto suo dentro il dispositivo e apre Impostazioni
→ Abbonamenti. Aggiungi il `.conf` della VPN e la playlist come sopra.
Limiti: niente registrazioni né pausa della diretta (sul dispositivo non c'è
ffmpeg); film, serie e diretta funzionano. Il dispositivo contatta la
playlist da sé, tramite la sua VPN.

### TV LG (webOS)

Le TV LG installano app esterne allo store solo in **Modalità
sviluppatore**. Se è troppo complicato, usa il browser della TV (sopra).

1. Crea un account gratuito su [webostv.developer.lge.com](https://webostv.developer.lge.com)
   (è il tuo account LG).
2. Sulla TV installa **Developer Mode** dall'LG Content Store, entra, attiva
   *Dev Mode Status* (la TV si riavvia), poi attiva *Key Server*. Annota
   l'indirizzo IP della TV e la passphrase che mostra.
3. Da un computer con Node.js nella stessa rete:

   ```sh
   npm install -g @webos-tools/cli
   ares-setup-device -a tv -i "{'host':'<ip-tv>','port':'9922','username':'prisoner'}"
   ares-novacom --device tv --getkey          # scrivi la passphrase
   ares-setup-device -m tv -i "{'privatekey':'tv_webos','passphrase':'<passphrase>'}"
   ares-install --device tv app.vodmax.client_<v>_all.ipk
   ```

   (`tv_webos` è il file della chiave che `--getkey` salva in `~/.ssh`.)
4. Apri VodMax sulla TV, scrivi l'indirizzo del server e approva il codice di
   abbinamento.

La Modalità sviluppatore dura **50 ore**: prima che scada apri l'app
Developer Mode e premi *Extend*, altrimenti le app installate così non si
aprono finché non la rinnovi. L'interfaccia completa richiede webOS 22 (TV
dal 2022) o successivo; sulle TV più vecchie funziona con un'impaginazione
meno curata.

### Samsung e altre TV

Non c'è ancora un'app Samsung (Tizen): usa il browser della TV su
`http://<server>:8095`.

## 4. Problemi comuni

| Problema | Cosa fare |
|---|---|
| "Il server non risponde" su un dispositivo | Stessa rete? Prova l'indirizzo dal browser del telefono. Controlla `systemctl status vodmax`. |
| Il codice di abbinamento è scaduto | Dura 10 minuti, poi ne compare uno nuovo. Una volta approvato, il dispositivo lo ritira entro 30 minuti, anche se l'app era in background. |
| La cartella di una raccolta "non esiste" | L'utente `vodmax` deve poterla leggere e, con il file di servizio fornito, non deve stare sotto `/home` o `/tmp`. |
| La diretta non parte, "connessioni in uso" | La playlist permette poche connessioni insieme: chiudi un altro dispositivo, o prendi il controllo. |
| La sincronizzazione fallisce | Impostazioni → VPN: la VPN è collegata? Impostazioni → Abbonamenti mostra l'errore. |
| Film MKV o AVI non partono nel browser | Installa `ffmpeg` sul server, o usa l'app Android. |
| Log | `journalctl -u vodmax -f` |
