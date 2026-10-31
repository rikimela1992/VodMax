# 0029 — Accesso con account di casa (OpenID Connect)

Stato: **Approvata** (31 ottobre 2026: "inizia a fare tutti i punti che hai
elencato prima")

## Decisione

- Chi ha già un provider di accesso (Authelia, Authentik, Keycloak, Google…)
  può far entrare i browser con un account invece del codice di
  abbinamento. Spento di default; si configura in Impostazioni →
  Dispositivi.
- Flusso *authorization code* con PKCE (S256), `state` e `nonce` monouso
  (10 minuti). Il provider deve essere in `https://`, anche i suoi
  indirizzi di autorizzazione e di token.
- L'ID token arriva direttamente dal token endpoint su TLS: come permette
  OpenID Connect (Core 3.1.3.7) si controllano emittente, destinatario,
  scadenza e nonce, non la firma. Niente librerie esterne né chiavi JWKS.
- **Nessun account entra da solo**: l'amministratore collega ogni account
  (e-mail verificata, oppure nome utente) a un profilo. Chi entra ottiene un
  dispositivo nuovo, bloccato su quel profilo, senza PIN (l'account è già la
  prova); si vede e si revoca tra i dispositivi abbinati.
- Al ritorno il browser riceve solo un codice monouso (2 minuti) nel
  frammento dell'indirizzo, che non finisce nei log; i token si ritirano con
  una POST.
- Il client secret resta cifrato sul server e non viene mai mostrato.

## Non ancora

- Creazione automatica dei profili e ruoli dai gruppi del provider.
- Accesso con account dalle app Android e dalla TV (lì resta il codice).
