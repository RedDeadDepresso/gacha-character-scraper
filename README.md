# kkafio-gacha-index

- `config/games.json` – gacha names, most popular first (drives ordering in the merged index).
- `config/commits.json` – last scraped repo commit per game (Blue Archive, Azur Lane, Zenless Zone Zero, Wuthering Waves, Arknights Endfield, Honkai Star Rail, Neverness to Everness, Nikke, Fate Grand Order, Genshin Impact, Girls Frontline 2, Pokemon and Hololive are skipped when their source repo is unchanged).
- `scrapers/` – one scraper per gacha (Arknights, Blue Archive, Azur Lane, Uma Musume, Zenless Zone Zero, Wuthering Waves, Arknights Endfield, Honkai Star Rail, Neverness to Everness, Nikke, Fate Grand Order, Genshin Impact, Girls Frontline 2, Pokemon Masters, Hololive). Register new ones in `main.go`.
- `data/<Gacha>.json` – scraped characters, newest first.
- `go run .` – run all scrapers locally (incremental: stops at the first already-known character). The wiki blocks GitHub's IPs, so run this on your own machine and commit the updated `data/*.json`.
- `go run ./cmd/merge` – merge `data/*.json` into `dist/gacha_character_index.json` (KKAFIO format).
- `.github/workflows/build-index.yml` – merges `data/*.json` and publishes to GitHub Pages on push or manual run (no scraping in CI).

The merge step folds alter versions (`X the Y`, `X (Y)` or `X • Y`, where `X` exists in the same series) into the base character `X`, adding the alter's name and aliases to `X`'s aliases.

`data/Gakuen Idolmaster.json` and `data/Project Sekai.json` are maintained by hand (no scraper, since new characters are rare). Keep it newest-first like the scraped files.