# 0013 — Migrazione affiancata dell'impianto di casa

Stato: **Approvata** (29 ottobre 2026)

## Contesto

Oggi a casa il servizio è diviso fra più componenti: gateway IPTV (provider,
account, playlist, EPG, diretta, pannello, prenotazioni automatiche),
Tvheadend (registrazioni dei canali IPTV, nessun sintonizzatore fisico),
Replay (riproduzione registrazioni, conversione HEVC, copie), Jellyfin
(metadati e copertine TMDB, player sulla TV webOS) e il VodMax di
laboratorio (interfaccia). L'obiettivo è che il nuovo VodMax li sostituisca
tutti.

## Decisione

**Configurazione affiancata.** Il nuovo VodMax gira accanto all'impianto
esistente, che continua a servire la famiglia. Un componente vecchio si
spegne **solo dopo** aver verificato che la sua controparte nel nuovo VodMax
funziona correttamente.

## Regole

1. Il nuovo VodMax non scrive mai nei sistemi vecchi e non dipende da loro
   per funzionare.
2. Le due installazioni non si contendono il provider: il nuovo VodMax usa
   un account o un numero di connessioni assegnato esplicitamente, sempre
   tramite la propria VPN interna (0012).
3. Per ogni componente da sostituire si scrive una **lista di parità**
   (cosa fa oggi, come si verifica nel nuovo) prima di iniziare.
4. Spegnimento in due tempi: prima **fermo ma ripristinabile**
   (servizio disattivato, dati intatti) per un periodo di prova, poi
   rimozione.
5. Ogni spegnimento è una decisione esplicita dell'utente.

## Fasi proposte

| Fase | Il nuovo VodMax impara a… | Controparte da spegnere dopo la verifica |
|---|---|---|
| 1 | Abbonamenti, pool di account, VPN interna, catalogo, EPG, copertine TMDB | Generazione playlist/STRM/NFO ed export del gateway |
| 2 | Diretta e riproduzione film/serie; interfaccia web e Android | VodMax di laboratorio |
| 3 | Registrazioni con regole (squadre, F1), replay, conversione HEVC | Tvheadend, Replay |
| 4 | App per TV webOS | Jellyfin |
| 5 | Pannello di amministrazione completo | Gateway (o ridotto a sola uscita di rete) |

L'ordine delle fasi è una proposta: va confermato dall'utente.

## Note

Questa scheda riguarda l'installazione di casa. Il codice del repository resta
generico: nessun indirizzo, provider o percorso dell'impianto di casa entra
nel progetto.
