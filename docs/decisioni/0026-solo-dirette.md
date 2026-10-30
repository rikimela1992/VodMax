# 0026 — Registrare solo le dirette

Stato: **Approvata** (30 ottobre 2026: "Proviamo", sulla proposta in tre
passi: indizi della guida, calendario reale, assistente LLM locale facoltativo)

## Problema

Una regola "tutte le partite della Juve" registra anche le repliche, le
differite, gli highlights e le partite storiche: la guida le chiama quasi
sempre allo stesso modo della diretta.

## Decisione — passo 1 (fatto)

- Opzione **Solo dirette** nella regola. Per ogni programma che la regola
  troverebbe, il server stima se è in diretta e perché; l'anteprima mostra
  il motivo, la regola salta le repliche ("N repliche saltate").
- Indizi, nell'ordine (il primo che decide vince):
  1. la guida XMLTV: `<live/>` (non nel DTD, usato da alcuni generatori),
     `<previously-shown/>`;
  2. parole intere del titolo: replica, differita, highlights, sintesi,
     best of, amarcord, rivivi, partita storica, replay, rerun…;
     della descrizione solo frasi sul modo di trasmissione ("in differita",
     "replica della", "già trasmessa"…);
  3. un anno nel titolo prima dell'anno scorso ("Juve - Inter 2006"; la
     stagione "2025/26" vale ancora);
  4. il canale: "+1", "Classic", "Story", "Storia", "Timeshift";
  5. "LIVE:" / "Diretta:" all'inizio del titolo, o "(Live)" alla fine:
     diretta (Sky li usa per programmi in diretta ogni giorno con lo stesso
     nome);
  6. se un'altra messa in onda dello stesso programma ha quel segno e
     questa no: replica;
  7. lo stesso titolo (e la stessa descrizione, se entrambi ne hanno una)
     già in onda da 90 minuti a 4 giorni prima; per un titolo in onda più
     di 6 volte ("Calcio Serie A") solo entro 20 ore e con la stessa
     descrizione;
  8. "diretta", "in diretta", "live", "dal vivo" nel titolo o nella
     descrizione; `<premiere/>` / `<new/>` della guida.
- Sinonimi nelle parole delle regole: "Formula 1" ↔ "F1", "MotoGP" ↔
  "Moto GP"; senza parole la regola cerca il suo nome.
- Senza indizi il programma si registra: perdere una diretta costa più che
  registrare una replica.

## Passi successivi (proposti)

2. **Calendario reale** delle partite (es. TheSportsDB, football-data.org):
   una partita è in diretta solo se l'orario coincide con quello ufficiale.
3. **Assistente LLM locale facoltativo** (Ollama / llama.cpp, API
   compatibile OpenAI, indirizzo nelle impostazioni, vuoto = spento): per
   scrivere le regole a parole e per i casi dubbi. Nessun modello dentro
   VodMax; il servizio gira su un'altra macchina della casa.
