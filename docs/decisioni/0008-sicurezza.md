# 0008 — Sicurezza, accesso e privacy

Stato: **Proposta**

## Proposta

- **Credenziali delle sorgenti** cifrate a riposo (Android Keystore
  sull'Agent; chiave in un file protetto sul Server). Mai nei log, mai nelle
  risposte API, mai nel browser.
- **Accesso al Server** solo dopo abbinamento del dispositivo (codice breve
  mostrato sul Server o su un dispositivo già abbinato). Token per dispositivo,
  revocabile.
- **Rete**: per default il Server ascolta solo sulla rete locale. L'accesso da
  fuori casa si lascia a strumenti dedicati (VPN, tunnel); niente esposizione
  diretta su internet.
- **Nessuna telemetria.** Nessuna chiamata a servizi esterni oltre a quelle
  configurate dall'utente (sorgenti, TMDB).
- **Repository**: scansione automatica dei segreti a ogni push (gitleaks);
  nessun file di configurazione reale, solo esempi con valori fittizi.

## Domande aperte

- Serve un account amministratore separato dai profili?
