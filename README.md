# gacha-character-scraper

- `config.json` – gacha names, most popular first (drives ordering in the merged index).
- `scrapers/` – one scraper per gacha (`Arknights` for now). Register new ones in `main.go`.
- `data/<Gacha>.json` – scraped characters, newest first.
- `go run .` – run all scrapers (incremental: stops at the first already-known character).
- `go run ./cmd/merge` – merge `data/*.json` into `dist/gacha-character-index.json` (KKAFIO format).
- `.github/workflows/build-index.yml` – scrapes weekly, merges, publishes to GitHub Pages.
