# 0009 — Licenza e pubblicazione

Stato: **Approvata** (30 ottobre 2026): **AGPL-3.0 con licenza commerciale**
per chi vuole un uso chiuso; repository pubblico.

## Decisione

- Licenza **AGPL-3.0** per tutto il progetto (server, interfaccia, app).
- **Doppia licenza**: chi vuole includere VodMax in un prodotto o servizio
  senza pubblicarne il codice può chiedere all'autore una licenza
  commerciale.
- Per poterla offrire l'autore deve avere i diritti su tutto il codice: i
  contributi esterni si accettano con il **CLA** di `CONTRIBUTING.md`.
- Più avanti: valutare la registrazione del marchio "VodMax".
- Controlli fatti prima della pubblicazione: cronologia Git completa (nessun
  segreto, indirizzo di casa o nome di provider; gitleaks nella CI),
  dipendenze con licenze compatibili, avviso sui contenuti e attribuzione
  TMDB nel README e nelle Impostazioni.

## Situazione prima della decisione

Repository **privato** finché la prima versione non è pronta e controllata.
Senza un file di licenza, tutti i diritti restano riservati.

## Opzioni di licenza

| Licenza | In breve |
|---|---|
| **GPL-3.0** | Chi distribuisce una versione modificata deve pubblicarne il codice. Protegge il progetto da fork chiusi. |
| **AGPL-3.0** | Come GPL, ma l'obbligo vale anche per chi lo offre come servizio in rete. Adatta a un server. |
| **MIT / Apache-2.0** | Chiunque può riusarlo, anche in prodotti chiusi. Massima diffusione. |

Raccomandazione: **GPL-3.0** per tutto il progetto, se l'obiettivo è che
resti aperto; **Apache-2.0** se si vuole la massima libertà di riuso.

## Prima di rendere pubblico

- Controllo segreti su tutta la cronologia Git, non solo sull'ultimo commit.
- Nessun riferimento a provider, indirizzi o percorsi della rete di casa.
- Dipendenze con licenze compatibili.
- README con avviso: VodMax non fornisce contenuti; l'utente è responsabile
  delle sorgenti che aggiunge.
- Attribuzione TMDB, se si usa l'arricchimento dei metadati.

## Domande aperte

- Quale licenza?
- Nome definitivo: "VodMax" va verificato come marchio prima della
  pubblicazione.
