# 0005 — Dati, profili e sincronizzazione

Stato: **Proposta**

## Proposta

- **Ogni nucleo ha il proprio SQLite.** L'Agent autonomo tiene tutto in
  locale; il Server tiene i dati di casa.
- **Profili** (persona) separati dai **dispositivi**. Un profilo ha lista,
  progressi, cronologia, preferenze e tema. Un dispositivo si abbina a un
  profilo con un codice breve; l'abbinamento si rinnova con l'uso e scade solo
  se il dispositivo resta inutilizzato (lezione dal VodMax precedente, dove la
  scadenza fissa a un anno avrebbe fatto perdere i profili).
- **Collegato → Server**: il dispositivo legge il catalogo dal Server e invia
  progressi e preferiti. Le modifiche fatte offline si mettono in coda e
  si inviano al ritorno; in caso di conflitto vince la più recente per singolo
  titolo.
- **Agent autonomo che si collega per la prima volta**: può caricare sul
  Server progressi e lista locali (con conferma).

## API

- HTTP + JSON, versionata (`/api/v1/...`), documentata con OpenAPI.
- Paginazione a cursore per le liste lunghe; mai "carica tutto".
- Aggiornamenti in tempo reale (progressi da un altro dispositivo, stato
  registrazioni) via Server-Sent Events.

## Domande aperte

- Profilo bambini con filtro per classificazione?
- PIN per profilo, o solo per un'area riservata come nel VodMax precedente?
