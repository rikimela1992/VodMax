# 0030 — Conversione video con la scheda grafica

Stato: **Approvata** (31 ottobre 2026: "inizia a fare tutti i punti che hai
elencato prima")

## Decisione

- Si converte solo il video che non è H.264 (HEVC, MPEG-2… aperti nel
  browser); tutto il resto si copia, come prima. Diretta, pausa e
  registrazioni non convertono il video.
- Impostazioni → Sistema → *Conversione video*: processore (libx264,
  predefinito), Intel/AMD (VAAPI, `h264_vaapi`), Intel Quick Sync (QSV,
  `h264_qsv`), NVIDIA (NVENC, `h264_nvenc`). Per VAAPI e QSV si sceglie il
  nodo in `/dev/dri/`.
- La pagina dice cosa manca (ffmpeg senza quell'encoder, scheda non visibile)
  e *Prova* converte 2 secondi di Full HD di prova mostrando l'errore vero.
- Decodifica col processore, codifica con la scheda: meno veloce della
  catena tutta in scheda, ma funziona con ogni formato in ingresso.
- **Se la scheda non parte si usa il processore**, nella stessa richiesta:
  chi guarda non vede errori.
- Immagine Docker: driver VAAPI Intel su x86-64 (36 MB); quello AMD (mesa)
  ne aggiungerebbe 165 per LLVM, quindi AMD solo con l'installazione nativa.
  La scheda si passa con `--device /dev/dri` (o il runtime NVIDIA).

## Non ancora

- Decodifica in scheda e ridimensionamento (per esempio 4K → 1080p).
- Verifica su hardware vero: in questo ambiente non c'è una scheda grafica.
