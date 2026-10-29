# 0012 — Connessioni VPN registrate

Stato: **Proposta**

## L'idea

L'utente registra una o più **connessioni VPN** e le associa agli
abbonamenti. Tutto il traffico di un abbonamento associato passa **solo**
dalla sua VPN.

## Proposta

- **Protocollo**: WireGuard come primo supporto (file `.conf` importabile o
  QR code, come lo forniscono Proton, Mullvad, NordVPN, Surfshark…).
  OpenVPN in seguito, se serve.
- **Associazione**: una VPN può servire più abbonamenti; un abbonamento ha al
  massimo una VPN. Abbonamenti senza VPN usano la connessione normale.
- **Blocco di sicurezza (kill switch) per abbonamento**: se la VPN associata
  non è attiva, quell'abbonamento non si collega affatto. Mai ripiego sulla
  connessione normale.
- **Stato visibile**: per ogni VPN, connessa/non connessa, IP di uscita,
  ultimo handshake; per ogni abbonamento, "via VPN X".

## Dove e come

| | Server | Agent Android |
|---|---|---|
| Meccanismo | Interfaccia WireGuard dedicata in un *network namespace* (o container affiancato), usata solo dai processi che parlano con quell'abbonamento | `VpnService` di Android con libreria WireGuard ufficiale, limitato alla sola app VodMax (`addAllowedApplication`) |
| Più VPN insieme | Sì, una per namespace | **No**: Android ammette una sola VPN attiva per volta |
| Altre VPN del dispositivo | Indipendenti | VodMax prende il posto di un'altra VPN attiva; va segnalato |
| Riproduzione | Il Server fa da passaggio, così i client non hanno bisogno della VPN | Il player dell'app passa dal tunnel |

Conseguenza pratica: con abbonamenti su VPN diverse, il dispositivo giusto
per gestirli è il **Server**, e gli Agent collegati riproducono attraverso di
lui.

## Servizi di streaming (famiglia B)

La VPN di VodMax non si applica a Netflix e agli altri servizi B: quei
contenuti si guardano nelle app ufficiali, che usano la rete del dispositivo.

## Sicurezza

- Chiavi private WireGuard cifrate a riposo, mai nei log né nelle API.
- Il traffico verso la VPN non deve mai uscire dalla connessione normale per
  errore: test automatici sul blocco di sicurezza.

## Domande aperte

- Serve anche OpenVPN fin dall'inizio?
- La VPN deve proteggere solo gli abbonamenti, o anche aggiornamenti di
  catalogo e copertine (TMDB)?
