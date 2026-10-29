# 0015 — Selezione del catalogo ("scremare" la lista del provider)

Stato: **Approvata nel principio** (29 ottobre 2026), prima versione
realizzata

## Contesto

La lista di un provider è inutilizzabile così com'è. Sul catalogo reale di
prova (29 ottobre 2026):

| | Dal provider | Note |
|---|---|---|
| Canali | 6.040 | 3.978 in 20 categorie estere "Live <paese>", 325 internazionali, 131 per adulti, 107 doppioni SD, 50 separatori finti |
| Film | 92.425 | 30.883 per adulti (360 categorie); ogni titolo presente in media 3 volte (qualità, lingua, categorie per attore) |
| Serie | 5.967 | per piattaforma d'origine |

La scelta di cosa tenere è una parte centrale del prodotto.

## Decisione

### 1. Classificazione automatica all'import

Per ogni **categoria**, dal nome:
- **paese**: "Live United States" → "United States"; "International …" →
  "Internazionale"; nessun prefisso → paese dell'abbonamento;
- **adulti** (`XXX`, `Adult`, `Porn`, `18+`, …);
- **doppioni di bassa qualità** ("SD (Connessioni Lente)");
- per i canali, **sezione**: **TV**, **Sport ed eventi** o **Radio**.

Per ogni **titolo o canale**:
- **qualità** della versione: 4K, FHD, HD o SD;
- **chiave di raggruppamento**: stesso titolo, ignorando maiuscole, accenti e
  tag di qualità, più l'anno per film e serie. "Rai 1 HD", "Rai 1 FHD" e
  "Rai 1 4K" diventano un'unica scheda; "Rai 1 +1" resta separato.

### 2. Regole dell'utente, per abbonamento

| Regola | Predefinito |
|---|---|
| Paesi esteri da tenere | nessuno: solo il paese dell'abbonamento |
| Contenuti per adulti | esclusi (vedi 0014) |
| Doppioni di bassa qualità | esclusi |
| Anno minimo film / serie | nessun limite |
| Escludere i titoli senza anno quando c'è un anno minimo | no |

### 3. Correzioni manuali, che valgono più delle regole

- Una **categoria** si può attivare o disattivare, e se ne possono cambiare
  sezione, paese e marcatura "adulti".
- Un **singolo titolo o canale** si può nascondere.
- Regole e correzioni sono salvate a parte e **sopravvivono a ogni
  sincronizzazione**.

### 4. Una scheda per titolo

Si mostra la versione migliore (qualità più alta); le altre sono
alternative ("versioni"). Il raggruppamento vale anche fra abbonamenti
diversi.

### 5. Anteprima

Prima di salvare, l'utente vede quanti canali, film e serie restano con le
nuove regole.

## Risultato sul catalogo reale (selezione predefinita)

| | Visibili | Schede |
|---|---|---|
| Canali | 1.441 | **1.067** (TV 389, Sport ed eventi 664, Radio 14) |
| Film | 61.542 | **20.625** |
| Serie | 5.967 | **5.922** |

Altri esempi: con film dal 1980, 19.315 schede; escludendo anche quelli senza
anno, 10.878. Anteprima in circa 260 ms, salvataggio in circa 700 ms su
104.000 elementi.

## Ancora da fare

- **Canali TV riconosciuti**: elenco dei canali nazionali noti (Rai 1,
  Canale 5, …) con numerazione LCN, logo e ordine "da telecomando". Oggi la
  sezione TV è in ordine alfabetico e i canali principali si perdono fra
  regionali e canali tematici.
- **Prima configurazione guidata**: paese, anno minimo, adulti sì/no.
- **Lingua dei film** (doppiato, sottotitolato, originale), se il provider la
  indica.
