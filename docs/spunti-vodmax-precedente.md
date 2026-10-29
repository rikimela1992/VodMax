# Spunti dal VodMax precedente

Il VodMax di laboratorio (2026) resta un riferimento per grafica e
soluzioni. **Nessun file di codice viene copiato.**

## Da riprendere

### Grafica

- Superficie "cinema": sfondo quasi nero, hero a tutta larghezza con il
  backdrop del titolo, scrim a gradiente per leggere il testo.
- Poster verticali 2:3 grandi, righe scorrevoli con scroll-snap; schede
  larghe (backdrop) per "Continua" e Sport.
- Poster mancante: fondo neutro con il titolo vero, mai un cartellone
  generico.
- Righe personali vuote non occupano spazio.
- Temi con token di colore (Automatico, Sala, Notte, Velluto, Laguna,
  Matinée): un set di variabili per tema, accento unico, contrasto verificato.
- Font di sistema, nessuna risorsa da CDN, animazioni ridotte con
  `prefers-reduced-motion`.

### Soluzioni

- Titolo mostrato senza tag di qualità e senza anno duplicato, titolo
  originale conservato per la ricerca; attenzione ai titoli numerici
  (1917, 1984, Blade Runner 2049).
- Raggruppamento delle versioni dello stesso film (qualità, lingua) in
  un'unica scheda.
- "Nuovi episodi" per le serie seguite; "Nuovi arrivi" dalla prima comparsa
  osservata, non da date inventate.
- Suggerimenti spiegabili ("perché te lo proponiamo"), "Non mi interessa".
- Ricerca full-text con prefissi (SQLite FTS5).
- Import del catalogo "tutto o niente": un aggiornamento incompleto o con un
  calo anomalo mantiene il catalogo precedente.

## Da evitare

| Problema visto | Regola per il nuovo progetto |
|---|---|
| Un unico lock per tutto il database: import di 30 s blocca l'app | Letture concorrenti (WAL), import su tabelle di appoggio |
| Id calcolati dal percorso del file: copertine perse | Id da sorgente + id della sorgente |
| Titoli con "(anno)" inviati alla ricerca TMDB | Titolo pulito + anno separato |
| Tetti fissi (40.000 voci) che fanno sparire tutto se superati | Limiti dimensionati e degradazione parziale, mai totale |
| Scadenza fissa dei dispositivi | Scadenza che si rinnova con l'uso |
| Componenti legati a IP e percorsi di casa | Tutto in configurazione |
