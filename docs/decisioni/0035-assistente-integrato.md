# 0035 — Assistente integrato, servizi online e attivazione in evidenza

Stato: **Approvata** (31 ottobre 2026: "mi piace molto di più così")

Completa la [0034](0034-assistente.md): stesse funzioni, ma attivarle non
deve richiedere un altro container o dei comandi.

## Decisione

### Facoltativo, ma in evidenza

- L'assistente resta **spento di default** e nessuna funzione ne ha bisogno.
- In cima alla pagina principale delle Impostazioni (la lista delle
  sezioni) c'è un riquadro in evidenza **"Attiva l'assistente"**, con una
  riga su cosa fa e il pulsante che porta alla nuova sezione
  **Assistente**. Lo vede solo l'amministratore; "Non ora" lo riduce a una
  voce normale della lista. Quando l'assistente è attivo diventa una voce
  normale ("Assistente — attivo, <modello>").
- La sezione esce da Sistema e diventa una voce sua nel gruppo Server.
- **Spento non lascia tracce**: niente "Cosa guardo stasera" in Home, niente
  "Compila con l'assistente" nelle regole, e così ogni funzione futura
  (ricerca a parole, riassunti). Nessun pulsante grigio o messaggio "attiva
  per usare".

### Tre modi

1. **Integrato** (consigliato). VodMax guarda RAM e scheda video e propone
   solo i modelli che ci stanno (con la RAM richiesta mentre risponde).
   "Attiva" scarica motore e modello con una barra di avanzamento, poi fa la
   prova da solo. Ogni file ha la sua impronta SHA-256 scritta in VodMax.
   Modelli scelti (tutti Apache 2.0, GGUF Q4_K_M da Hugging Face, provati
   sulla guida demo):

   | Scelta | Modello | File | Prova |
   |---|---|---|---|
   | Leggero | Qwen 2.5 1.5B Instruct | 0,9 GB | regole buone, consigli della sera confusi: detto nella scheda |
   | Bilanciato | Qwen 3 4B Instruct 2507 | 2,3 GB | regole e consigli corretti, in italiano: il consigliato |
   | Qualità | Qwen 2.5 14B Instruct | 8,4 GB | il migliore; consigliato solo con scheda video |

2. **Servizio online**: fornitore da un elenco e chiave API (cifrata, mai
   più mostrata). **Claude (Anthropic)** con la sua API nativa, modello
   predefinito Claude Haiku 5.5 con sforzo basso (SDK ufficiale Anthropic
   per Go); OpenAI, Mistral, Gemini e OpenRouter già impostati. "Elenco
   modelli" chiede al servizio i modelli disponibili con la chiave. Una riga
   spiega che serve una **chiave API, non l'abbonamento** (Claude Pro/Max, ChatGPT Plus non si possono usare da
   un'app esterna), con il link per crearla, e che guida e titoli escono
   da casa.
3. **Server tuo**: l'indirizzo libero di oggi (Ollama, LM Studio, qualsiasi
   API compatibile OpenAI).

### Come funziona l'integrato

- Motore: **llama.cpp** (`llama-server`, API compatibile OpenAI), versione
  fissata per ogni release di VodMax con la sua impronta. Pacchetti ufficiali
  per Linux: CPU ~21 MB, Vulkan ~34 MB (schede Intel, AMD e NVIDIA senza
  CUDA), arm64 ~17 MB.
- VodMax lo avvia come processo figlio in ascolto **solo su 127.0.0.1**, su
  una porta scelta da lui: nessuna porta da aprire, nessuna password.
- Il modello resta in memoria finché serve e si spegne dopo 10 minuti senza
  richieste; la prima richiesta dopo una pausa lo riaccende (qualche
  secondo in più).
- File in `<dati>/assistant/`. "Rimuovi" cancella motore e modello e libera
  lo spazio.
- Download solo da GitHub (motore) e Hugging Face (modello). Nessuna
  richiesta al provider IPTV, nessun dato della casa nelle richieste.

### Non pesa sull'immagine

- Il pacchetto VodMax e l'immagine Docker **non contengono** né motore né
  modelli: si scaricano solo premendo "Attiva".
- L'immagine Docker è Alpine (musl) e i pacchetti di llama.cpp sono per
  glibc: in Docker e Unraid l'integrato usa un servizio facoltativo nel
  `docker-compose` (Ollama, profilo `assistant`), che VodMax trova con
  `VODMAX_ASSISTANT_OLLAMA` e a cui chiede di scaricare gli stessi file GGUF,
  con la stessa barra e gli stessi pulsanti. Chi non lo attiva non scarica
  niente. Su Unraid la stessa variabile punta a un container Ollama.
- Il programma VodMax cresce di circa 5 MB (da 18 a 23 MB) per l'SDK di
  Claude; nessun'altra dipendenza.

## Non ancora

- Le altre idee della 0034 (ricerca a parole, "nella puntata precedente",
  pulizia, diagnosi, avvisi).
- Scegliere da soli il backend CUDA/ROCm: per ora CPU o Vulkan.
