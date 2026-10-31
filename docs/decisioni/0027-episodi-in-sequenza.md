# 0027 — Prossimo episodio, salta intro e salta finale

Stato: **Approvata** (31 ottobre 2026: "manca anche un prossimo episodio,
non va avanti da solo nell'episodio successivo e non c'è neanche un salta
intro o outro")

## Decisione

- Durante un episodio: pulsante **Prossimo episodio** tra i comandi; verso
  la fine un riquadro "Prossimo episodio · tra N s" con **Guarda ora** e
  **Annulla**; a fine episodio parte da solo il successivo (anche alla
  stagione dopo), se non si è premuto Annulla.
- **Salta intro**: compare mentre scorre la sigla; con il telecomando ha
  il fuoco, quindi basta OK.
- I provider non dicono dove sono sigla e titoli di coda, e analizzare
  l'audio di ogni episodio costerebbe troppo: VodMax **li impara da chi
  guarda**, per serie, e li tiene sul server (tabella `series_marker`,
  `PUT /api/v1/series/{id}/markers`), così valgono su ogni dispositivo:
  - salti in avanti ravvicinati (entro 8 s l'uno dall'altro) che partono
    nei primi 5 minuti e saltano da 20 s a 10 minuti segnano la sigla;
  - **Prossimo episodio** premuto a 15 s – 10 minuti dalla fine segna la
    durata dei titoli di coda: dagli episodi dopo il riquadro compare lì
    e il successivo parte 10 s dopo ("salta finale").
  - Finché non sono noti: niente Salta intro, riquadro 15 s prima della
    fine e passaggio al successivo alla fine vera.
- Stesso comportamento nel player del browser/TV LG e in quello nativo
  dell'app Android (Media3), che restituisce all'interfaccia cosa ha
  imparato e se passare al successivo.

## Correzione nell'app Android (31 ottobre 2026)

Segnalazione: sul telefono, nelle serie, toccando lo schermo comparivano
solo la barra del tempo e "Prossimo episodio", senza ±10/30 s e pausa. Il
pulsante "Prossimo episodio" era un elemento nostro sovrapposto ai comandi
di Media3 e aggiornato ogni mezzo secondo, mentre Media3, nascondendo i
comandi, passa per uno stato con la sola barra. Ora "Prossimo episodio" è il
pulsante ⏭ dei comandi stessi (un `ForwardingPlayer` dichiara il comando "vai
al successivo") e i comandi compaiono e spariscono tutti insieme. Restano
sovrapposti solo "Salta intro" e il riquadro dei titoli di coda, che
compaiono quando servono. Verificata la compilazione; la prova sul telefono
resta da fare.
