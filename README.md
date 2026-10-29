# VodMax

Media center in stile Plex / Jellyfin / IPTV Extreme: aggiungi i tuoi
**abbonamenti** (provider IPTV, playlist M3U, file locali, ma anche Netflix,
Prime Video, Disney+, Paramount+…) e ritrovi tutto in una sola Home, una sola
ricerca, una sola lista. Ogni abbonamento può passare da una **VPN**
registrata.

Un'unica applicazione che funziona da sola su un dispositivo (**Agent**) e
che può diventare il **Server** della casa per gli altri dispositivi.

> Stato: **progettazione**. Il repository contiene per ora soltanto le
> decisioni di base, da approvare prima di scrivere codice.

## L'idea in breve

| Modalità | Dove gira | Cosa fa |
|---|---|---|
| **Agent** | Android (telefono, TV box, Android TV) | Player autonomo come IPTV Extreme / TiviMate: aggiungi una sorgente (M3U, Xtream Codes, EPG XMLTV) e guardi. Nessun server necessario. |
| **Agent → Server** | Lo stesso Android, acceso in casa | Con un interruttore l'Agent espone catalogo, profili, progressi ed EPG agli altri dispositivi della rete. |
| **Server** | Linux / Docker / NAS | Stessa logica, senza interfaccia locale: sempre acceso, registrazioni, più utenti, interfaccia web. |

Un dispositivo che trova un Server in rete può collegarsi a lui invece di
gestire le sorgenti da solo: profili, preferiti e "Continua a guardare"
diventano condivisi.

## Abbonamenti

| Famiglia | Tipi | Cosa fa VodMax |
|---|---|---|
| **Riproducibili** | Xtream Codes, M3U + EPG, file locali (Server), in seguito Jellyfin/Plex e Tvheadend | Riproduce nel proprio player, con progressi, registrazioni e VPN |
| **Servizi di streaming** | Netflix, Prime Video, Disney+, Paramount+, RaiPlay… | Mostra i titoli disponibili nel catalogo unico e con **Guarda** apre l'app ufficiale sul titolo |

I servizi di streaming sono protetti da DRM: VodMax non li riproduce, non ne
chiede le password e non li registra. Dettagli nella
[decisione 0011](docs/decisioni/0011-abbonamenti-e-tipi-di-account.md);
VPN nella [decisione 0012](docs/decisioni/0012-connessioni-vpn.md).

## Principi

- **Nessun contenuto incluso.** VodMax è un player: le sorgenti le aggiunge
  l'utente. Nessun provider, URL o credenziale nel codice.
- **Veloce con cataloghi enormi.** Obiettivo: Home e liste sotto i 300 ms con
  oltre 200.000 elementi (il caso che oggi rende lento Jellyfin).
- **Funziona offline dal server.** L'Agent non dipende mai da un Server per
  guardare le proprie sorgenti.
- **Un solo nucleo.** Agent e Server condividono la stessa logica; cambia solo
  chi la ospita.

## Documentazione

- [Decisioni di base](docs/decisioni/README.md): una scheda per ogni scelta,
  con opzioni, raccomandazione e domande aperte.
- [Spunti dal VodMax precedente](docs/spunti-vodmax-precedente.md): cosa
  riprendere (grafica e soluzioni) e cosa evitare. Nessun codice riutilizzato.

## Licenza

Da decidere prima della pubblicazione (vedi
[decisione 0009](docs/decisioni/0009-licenza-e-pubblicazione.md)). Finché il
repository è privato, tutti i diritti sono riservati.
