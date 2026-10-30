# Contributing to VodMax

Thanks! Bug reports and ideas: open an
[issue](https://github.com/silirim/VodMax/issues). For security
vulnerabilities read [SECURITY.md](SECURITY.md).

## Before a pull request

- `gofmt`, `go vet ./...` and `go test -race ./...` must pass (CI checks
  them too, together with gitleaks).
- No real data: no provider addresses, credentials, playlists or private
  network addresses. Tests use fake servers and generated credentials.
- Important choices have a page in [docs/decisioni](docs/decisioni)
  (Italian).

## Contributor License Agreement (CLA)

VodMax is distributed under the AGPL-3.0 and, on request, under a
commercial license (see the [README](README.md#license)). To do that the
author must hold the rights to all the code. By sending a contribution
(pull request, patch or other) you declare that:

1. the contribution is your own work, or you have the right to send it
   under these terms (for example from your employer);
2. it remains yours, and you grant the author of VodMax (silirim) a
   **perpetual, worldwide, royalty-free, non-exclusive and irrevocable**
   license to use, modify, sublicense and distribute it under any license,
   commercial licenses included;
3. you grant the same license on any of your patents needed to use the
   contribution;
4. the contribution will in any case be available to everyone under the
   AGPL-3.0.

Tick the CLA box in the pull request. Without it the pull request cannot
be accepted.

---

## Contribuire a VodMax (italiano)

Grazie! Segnalazioni e proposte: apri una
[issue](https://github.com/silirim/VodMax/issues). Per le vulnerabilità
di sicurezza leggi [SECURITY.md](SECURITY.md).

### Prima di una pull request

- `gofmt`, `go vet ./...` e `go test -race ./...` devono passare (lo
  controlla anche la CI, insieme a gitleaks).
- Nessun dato reale: niente indirizzi di provider, credenziali, playlist o
  indirizzi di rete privati. I test usano server finti e credenziali generate.
- Le scelte importanti hanno una scheda in [docs/decisioni](docs/decisioni).

### Accordo di contribuzione (CLA)

VodMax è distribuito con l'AGPL-3.0 e, a chi lo chiede, con una licenza
commerciale (vedi il [README](README.it.md#licenza)). Per poterlo fare l'autore
deve avere i diritti su tutto il codice. Inviando un contributo (pull
request, patch o altro) dichiari che:

1. il contributo è opera tua, oppure hai il diritto di inviarlo con queste
   condizioni (per esempio dal tuo datore di lavoro);
2. resta tuo, e concedi all'autore di VodMax (silirim) una licenza
   **perpetua, mondiale, gratuita, non esclusiva e irrevocabile** di usarlo,
   modificarlo, sublicenziarlo e distribuirlo con qualsiasi licenza, comprese
   licenze commerciali;
3. concedi la stessa licenza su eventuali tuoi brevetti necessari per usare
   il contributo;
4. il contributo sarà comunque disponibile a tutti con l'AGPL-3.0.

Nella pull request spunta la casella del CLA. Senza, la pull request non
può essere accettata.
