# kkafio-gacha-index

- `config/games.json` – gacha names, most popular first (drives ordering in the merged index).
- `config/commits.json` – last scraped SchaleDB commit (Blue Archive is skipped when unchanged).
- `scrapers/` – one scraper per gacha (Arknights, Blue Archive). Register new ones in `main.go`.
- `data/<Gacha>.json` – scraped characters, newest first.
- `go run .` – run all scrapers locally (incremental: stops at the first already-known character). The wiki blocks GitHub's IPs, so run this on your own machine and commit the updated `data/*.json`.
- `go run ./cmd/merge` – merge `data/*.json` into `dist/gacha_character_index.json` (KKAFIO format).
- `.github/workflows/build-index.yml` – merges `data/*.json` and publishes to GitHub Pages on push or manual run (no scraping in CI).

The merge step folds alter versions (`X the Y` or `X (Y)`, where `X` exists in the same series) into the base character `X`, adding the alter's name and aliases to `X`'s aliases.
