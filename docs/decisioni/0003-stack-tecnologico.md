# 0003 — Stack tecnologico

Stato: **Proposta**

## Vincolo principale

Lo stesso nucleo deve girare **dentro un'app Android** (Agent) e **su un
server Linux** (Server), senza riscriverlo.

## Opzione A — Kotlin ovunque (raccomandata)

- **Nucleo**: Kotlin Multiplatform (modulo condiviso JVM + Android).
- **Server HTTP**: Ktor, lo stesso sia nel Server sia nell'Agent promosso.
- **Database**: SQLite tramite SQLDelight (query SQL scritte a mano,
  verificate in compilazione, stesse su Android e JVM).
- **App Android**: Jetpack Compose (telefono) + Compose for TV (televisore),
  player Media3 / ExoPlayer.
- **Server**: JVM in container Docker; un solo processo.

Pro: un linguaggio, un nucleo, Agent → Server immediato. Media3 è il player
migliore su Android. Contro: la JVM sul server usa più memoria di Go o Rust
(~150–250 MB), accettabile.

## Opzione B — Go per nucleo e server, Kotlin per l'app

Nucleo in Go, incluso nell'app con gomobile. Server molto leggero.
Contro: il confine Go ↔ Kotlin rende più difficili interfaccia e debug;
due linguaggi da mantenere.

## Opzione C — Rust per il nucleo

Massime prestazioni, nucleo incluso via FFI. Contro: curva di apprendimento
e tempi di sviluppo più alti.

## Vincolo aggiunto dalla 0012 (VPN interna)

La VPN interna richiede WireGuard in user space con stack di rete proprio.
Le implementazioni mature sono in **Go** (`wireguard-go` + gVisor, usate da
Tailscale e `wireproxy`). Conseguenze:

- con l'opzione A (Kotlin) il tunnel diventa una piccola libreria Go inclusa
  sia nel Server sia nell'app (gomobile), che espone al resto del codice solo
  "apri una connessione verso questo host";
- l'opzione B (Go per nucleo e server) diventa più interessante, perché il
  tunnel sarebbe nello stesso linguaggio del nucleo.

La scelta fra A e B va rivalutata alla luce di questo vincolo.

## Interfaccia web

Separata dal nucleo e servita da Server e Agent promosso: TypeScript con
un framework leggero (Preact o Svelte), senza CDN esterni. La stessa interfaccia
può diventare l'app per **webOS** (vedi 0004).

## Database: regole per non ripetere i problemi di Jellyfin

- Letture su connessioni separate (WAL), mai un unico lock per tutto.
- Import del catalogo su tabelle di appoggio, poi scambio in un'unica
  transazione breve.
- Query della Home misurate da test automatici su un catalogo sintetico da
  200.000 elementi, con limite di tempo.

## Domande aperte

- Va bene Kotlin come linguaggio principale?
- Server solo Docker, o anche pacchetto `.deb` / servizio systemd?
