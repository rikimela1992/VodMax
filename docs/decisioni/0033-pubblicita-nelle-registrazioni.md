# 0033 — Pubblicità nelle registrazioni (Comskip)

Stato: **Approvata** (31 ottobre 2026: "Prima finiamo i punti mancanti del
task prima")

## Decisione

- Se sulla macchina c'è [Comskip](https://github.com/erikkaashoek/Comskip)
  (programma esterno, nel PATH) e l'opzione è accesa (Impostazioni →
  Sistema → Spazio delle registrazioni), a fine registrazione VodMax lo fa
  girare e Comskip scrive un file `.edl` accanto alla registrazione.
- **Non si taglia niente**: il lettore web mostra "Salta pubblicità" dentro
  ogni blocco, e OK porta alla fine del blocco. Un errore di Comskip costa
  un pulsante, non un pezzo di programma.
- Un'analisi alla volta, con un limite di 2 ore. Le impostazioni di Comskip
  si cambiano con `comskip.ini` nella cartella delle registrazioni.
- Il file `.edl` si cancella con la registrazione.

## Limiti

- L'immagine Docker non contiene Comskip (non è nei pacchetti Alpine): serve
  l'installazione nativa, o un'immagine propria.
- L'app Android con il lettore nativo non mostra ancora il pulsante.
- Provato con un Comskip finto nei test: in questo ambiente il programma
  vero non c'è.
