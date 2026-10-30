# 0025 — Download di film e serie sul server

Stato: **Approvata** (30 ottobre 2026: "aggiungiamo la parte di download dei
film o serie onpremise")

## Decisione

- Dalla scheda di un film o di una serie: **Scarica** (film), **Scarica la
  stagione N**, **Scarica la serie**. Profili bambini esclusi. Elenco con
  avanzamento, pausa, ripresa ed eliminazione nella pagina Registrazioni.
- I file vanno nella cartella dei download del server (`<data>/downloads`,
  opzione `-downloads`) con i nomi di una libreria Jellyfin/Kodi:
  `Movies/Titolo (Anno)/Titolo (Anno).ext`,
  `TV Shows/Serie (Anno)/Season N/Serie S01E02 - Titolo.ext`, con `.nfo` e
  locandina. Le due cartelle diventano da sole le raccolte **«Scaricati ·
  Film»** e **«Scaricati · Serie»** (decisione 0022): il titolo scaricato
  parte dal disco ed è la variante proposta per prima.
- **Una connessione sola**: un download alla volta e solo con una
  connessione libera; chi guarda la prende subito, senza domande, e il
  download riprende più tardi da dove si era fermato (richieste a
  intervalli). Opzione: solo in una fascia oraria (es. di notte).
- File originale del provider, senza conversione; spazio controllato
  prima di partire.
- Non sul dispositivo Android "da solo" (decisione 0021).
