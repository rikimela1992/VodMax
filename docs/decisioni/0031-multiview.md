# 0031 — Multiview: più canali insieme

Stato: **Approvata** (31 ottobre 2026: "inizia a fare tutti i punti che hai
elencato prima")

## Decisione

- Nuova pagina `multiview.html`, dal pulsante con la griglia nei controlli
  della TV: 2 o 4 canali insieme (sul telefono uno sotto l'altro).
- L'audio è quello del riquadro selezionato; gli altri sono muti. Frecce per
  scegliere il riquadro, OK per cambiarne il canale, Rosso per svuotarlo,
  Indietro per aprire il canale selezionato a schermo intero. Con il mouse:
  clic per selezionare, secondo clic per cambiare canale, doppio clic per lo
  schermo intero.
- **Nessun cambiamento nel server**: ogni riquadro è uno spettatore a sé
  (`<spettatore>-mv<n>`), con il suo permesso e la sua connessione. Il limite
  di connessioni dell'abbonamento vale come sempre: un riquadro senza
  connessione libera lo dice e gli altri continuano.
- Flusso diretto, senza pausa: il buffer su disco per quattro canali
  costerebbe quattro volte lo spazio.
- I canali scelti restano salvati sul dispositivo.

## Da verificare

- Quanti video insieme regge ogni TV: alcune decodificano solo uno o due
  flussi alla volta. Per questo c'è la scelta tra 2 e 4.
