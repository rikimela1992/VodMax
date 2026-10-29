# 0001 — Modalità Agent, Server e promozione Agent → Server

Stato: **Proposta**

## Contesto

Si vuole un'app che, come IPTV Extreme, funzioni da sola su un dispositivo,
ma che possa anche fare da server per la casa. Serve anche una versione
server pura, sempre accesa, senza schermo.

## Proposta

Un unico **nucleo** (catalogo, EPG, profili, progressi, ricerca, sorgenti)
ospitato in tre modi:

1. **Agent**: il nucleo gira dentro l'app Android e serve solo l'interfaccia
   locale. Nessuna porta aperta verso la rete.
2. **Agent promosso a Server**: un interruttore nelle impostazioni avvia un
   piccolo server HTTP dentro l'app. Gli altri dispositivi lo trovano in rete
   (mDNS / DNS-SD, `_vodmax._tcp`) e si abbinano con un codice.
3. **Server**: lo stesso nucleo come servizio Linux / container Docker, con
   interfaccia web e funzioni che richiedono di essere sempre acceso
   (registrazioni, aggiornamento notturno del catalogo).

Un dispositivo può essere:

- **Autonomo**: usa le proprie sorgenti;
- **Collegato**: usa catalogo, profili e progressi di un Server. Se il Server
  non risponde, torna alle proprie sorgenti, se ne ha.

## Alternative valutate

- **Solo server + client sottili** (modello Jellyfin/Plex): semplice da
  pensare, ma senza server non si guarda niente. Scartata come unico modello.
- **Solo app autonoma** (modello IPTV Extreme / TiviMate): ogni dispositivo
  rifà tutto da solo, nessuna condivisione. Scartata come unico modello.
- **Codice separato per Agent e Server**: doppio lavoro e comportamenti
  diversi. Scartata.

## Conseguenze

- Il nucleo non può dipendere da Android né dal sistema operativo del server
  (vedi 0003).
- Sull'Agent promosso il server deve rispettare batteria e memoria: limiti
  sui client collegati, niente transcodifica.
- Serve un protocollo stabile e versionato fra Collegato e Server (vedi 0005).

## Domande aperte

- L'Agent promosso deve funzionare anche con lo schermo spento (servizio in
  primo piano con notifica)?
- Il Server deve poter avere anche più Agent "satellite" che registrano
  (per esempio un TV box che ha il tuner)?
