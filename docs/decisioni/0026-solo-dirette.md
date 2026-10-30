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
  5. lo stesso titolo già in onda da 90 minuti a 4 giorni prima (la guida
     tiene 7 giorni di storia); un titolo in onda più di 6 volte nella
     finestra ("Calcio Serie A") non conta;
  6. "diretta", "in diretta", "live", "dal vivo" nel titolo o nella
     descrizione; `<premiere/>` / `<new/>` della guida.
- Senza indizi il programma si registra: perdere una diretta costa più che
  registrare una replica.

## Passi successivi (proposti)

2. **Calendario reale** delle partite (es. TheSportsDB, football-data.org):
   una partita è in diretta solo se l'orario coincide con quello ufficiale.
3. **Assistente LLM locale facoltativo** (Ollama / llama.cpp, API
   compatibile OpenAI, indirizzo nelle impostazioni, vuoto = spento): per
   scrivere le regole a parole e per i casi dubbi. Nessun modello dentro
   VodMax; il servizio gira su un'altra macchina della casa.
