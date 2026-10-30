# Contribuire a VodMax

Grazie! Segnalazioni e proposte: apri una
[issue](https://github.com/silirim/VodMax/issues). Per le vulnerabilità
di sicurezza leggi [SECURITY.md](SECURITY.md).

## Prima di una pull request

- `gofmt`, `go vet ./...` e `go test -race ./...` devono passare (lo
  controlla anche la CI, insieme a gitleaks).
- Nessun dato reale: niente indirizzi di provider, credenziali, playlist o
  indirizzi di rete privati. I test usano server finti e credenziali generate.
- Le scelte importanti hanno una scheda in [docs/decisioni](docs/decisioni).

## Accordo di contribuzione (CLA)

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
