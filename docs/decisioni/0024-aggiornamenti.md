# 0024 — Aggiornamenti

Stato: **Approvata** (30 ottobre 2026: "Procedi")

## Decisione

- **Controllo**: il server legge le release di GitHub una volta al giorno
  (`api.github.com`, nessun dato sull'installazione) e tiene la più recente,
  con i checksum (`SHA256SUMS`) di APK e archivi. Si disattiva da
  Impostazioni → Sistema → Versione e aggiornamenti. Le versioni di sviluppo
  (`dev`) non controllano.
- **Server**: non si sostituisce da solo (il servizio non può scrivere i
  programmi di sistema, apposta). `sudo vodmax-update` scarica l'archivio
  per l'architettura, lo verifica, tiene il precedente (`.old`), installa e
  riavvia; con una versione come argomento torna anche indietro.
- **App Android**: chiede al proprio server se c'è un APK più recente,
  lo scarica solo dalle release di `silirim/VodMax`, ne verifica lo SHA-256
  e apre l'installatore di Android: l'utente conferma (Android non permette
  aggiornamenti silenziosi fuori dagli store).
- **Firma**: gli APK di rilascio sono firmati con una chiave dedicata
  (RSA 4096, conservata fuori dal repository, con backup); Android accetta
  un aggiornamento solo con la stessa chiave. La 0.4.0 è la prima così: chi
  ha installato una versione precedente (chiave di debug) reinstalla una
  volta.
- **webOS**: l'interfaccia viene dal server ed è sempre aggiornata.
