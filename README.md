# kkafio-gacha-index

- `config/commits.json` – last scraped repo commit per game (Blue Archive, Azur Lane, Zenless Zone Zero, Wuthering Waves, Arknights Endfield, Honkai Star Rail, Neverness to Everness, Nikke, Fate Grand Order, Genshin Impact, Girls Frontline 2, Pokemon, Hololive, Granblue Fantasy and Punishing Gray Raven are skipped when their source repo is unchanged).
- `scrapers/` – one scraper per gacha (Arknights, Blue Archive, Azur Lane, Uma Musume, Zenless Zone Zero, Wuthering Waves, Arknights Endfield, Honkai Star Rail, Neverness to Everness, Nikke, Fate Grand Order, Genshin Impact, Girls Frontline 2, Pokemon Masters, Hololive, Granblue Fantasy, Punishing Gray Raven). Register new ones in `main.go`.
- `config/series.json` – the list of gachas, most popular first (drives ordering in the merged index and which scrapers run). Per game: `game` (scraper / data file name), the series `title` in each language (`en_us`, `zh_cn`, `zh_tw`, `ja_jp`, `ko_kr`, `ru_ru`; `en_us` defaults to the game name, so folder names stay stable) and `aliases`. Titles avoid characters that are invalid in folder names. A missing language falls back to `en_us` in KKAFIO.
- `data/<Gacha>.json` – scraped characters, newest first. `name` is an object keyed by language code (`{"en_us": "...", "ja_jp": "..."}`), `aliases` are other ways to call the character (short names, other word orders). Old files with a plain-string `name` still load; scrapers rebuild them in the new format on the next run even if the source repo is unchanged.
- `go run .` – run all scrapers locally (incremental: stops at the first already-known character). The wiki blocks GitHub's IPs, so run this on your own machine and commit the updated `data/*.json`.
- `go run ./cmd/merge` – merge `data/*.json` into `dist/gacha_character_index.json` (KKAFIO format).
- `.github/workflows/build-index.yml` – merges `data/*.json` and publishes to GitHub Pages on push or manual run (no scraping in CI).

The merge step folds alter versions (`X the Y`, `X (Y)` or `X • Y`, where `X` exists in the same series) into the base character `X`, adding the alter's name and aliases to `X`'s aliases.

`data/Gakuen Idolmaster.json` and `data/Project Sekai.json` are maintained by hand (no scraper, since new characters are rare). Keep it newest-first like the scraped files.

Index format: see KKAFIO's `06-gacha-index-format.md` (`title` / `name` per language, `"Last, First"` names). Most sources do not separate family and given names, so names are written as the source gives them; only Blue Archive (SchaleDB) is split into `"Family, Personal"` in every language. In `data/*.json` for hand-maintained games you can write `"Last, First"` names per language directly.
