#!/usr/bin/env python3
"""Merge data/*.json into a single gacha_character_index.json (KKAFIO index format).

For every game of config/series.json (most popular first) the characters of
data/<game>.json are read (newest first), alter versions are folded into their base
character, collab characters are left out and the characters are scored:

* a game with a `bangumi_id` uses the `score` the scraper saved (the Bangumi popularity,
  see `go run .`) and skips the characters flagged `collab`;
* a game without one is scored by its position in series.json (100, 90, 80 ... min 10),
  older characters higher.

The same character (same English name) in several games becomes one entry in several
series. Names and series titles that have a Simplified Chinese name (`zh_cn`) but no
Traditional one (`zh_tw`) get one through OpenCC (pip install OpenCC).

  python scripts/merge.py [--out dist/gacha_character_index.json]
"""

from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Callable
from pathlib import Path

LANG_EN, LANG_ZH, LANG_TW, LANG_JA, LANG_KO, LANG_RU = "en_us", "zh_cn", "zh_tw", "ja_jp", "ko_kr", "ru_ru"
OPENCC_CONFIG = "s2t"      # Simplified -> Traditional (use "s2tw" / "s2twp" for Taiwan wording)

Names = dict[str, str]
Char = dict   # data/<game>.json entry: name, aliases, score, collab, birthMonth, birthDay


def english(names: Names) -> str:
    """The English name, or the first available one (in a fixed order)."""
    for lang in (LANG_EN, LANG_JA, LANG_ZH, LANG_TW, LANG_KO, LANG_RU):
        if names.get(lang):
            return names[lang]
    return ""


def union(*lists: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for lst in lists:
        for s in lst or []:
            if s and s not in seen:
                seen.add(s)
                out.append(s)
    return out


def without_names(aliases: list[str], names: Names) -> list[str]:
    """Drop the aliases that equal one of the names (case-insensitively)."""
    skip = {n.lower() for n in names.values()}
    return [a for a in aliases if a.lower() not in skip]


def fold_names(names: Names, aliases: list[str], other: Char) -> tuple[Names, list[str]]:
    """Add the names of `other` to names / aliases: a language without a name takes other's,
    a different name in a language that has one becomes an alias, and other's aliases are
    added. Aliases equal to a name are dropped."""
    extra = list(other.get("aliases") or [])
    for lang in sorted(other["name"]):
        if lang not in names:
            names[lang] = other["name"][lang]
        elif names[lang].lower() != other["name"][lang].lower():
            extra.append(other["name"][lang])
    return names, without_names(union(aliases, extra), names)


# An alter version of a character: "Ch'en the Dawnstreak", "Amiya (Guardian)", "Himeko • Nova",
# "Nemesis: Gnosis" and "Nemesis·Gnosis" are alters of "Ch'en", "Amiya", "Himeko" and "Nemesis".
# The colon and the middle dots (· U+00B7, ・ U+30FB) match with or without spaces around them.
ALTER_SEPARATORS = (" the ", " (", " • ", ":", "・", "·")


def base_of(name: str, idx: dict[str, int]) -> str | None:
    """The lowercase English name of the base character if `name` looks like an alter of a
    character in idx (the current game's characters only), else None."""
    for sep in ALTER_SEPARATORS:
        for i in range(1, len(name) - len(sep) + 1):
            if name[i:i + len(sep)].lower() == sep:
                base = name[:i].strip().lower()
                if base in idx:
                    return base
    return None


def merge_alters(chars: list[Char]) -> list[Char]:
    """Fold alter versions into their base character (the older one), adding every name of the
    alter (all languages) and its aliases to the base's aliases. Only characters of the same
    list (one game) are merged; alters are recognised by their English name."""
    idx = {english(c["name"]).lower(): i for i, c in enumerate(chars)}

    def root(i: int) -> int:
        for _ in range(10):
            base = base_of(english(chars[i]["name"]), idx)
            if base is None or idx[base] == i:
                return i
            i = idx[base]
        return i

    out: list[Char] = []
    out_pos: dict[int, int] = {}
    for i, c in enumerate(chars):
        if root(i) == i:
            out_pos[i] = len(out)
            out.append({**c, "name": dict(c["name"]), "aliases": union(c.get("aliases") or [])})
    for i, c in enumerate(chars):
        r = root(i)
        if r != i:
            base = out[out_pos[r]]
            base["aliases"] = union(base["aliases"], c.get("aliases") or [], sorted(c["name"].values()))
    for c in out:
        c["aliases"] = without_names(c["aliases"], c["name"])
    return out


def load_chars(path: Path) -> list[Char]:
    """The characters of a data file (an old single-string name is read as the English one)."""
    out = []
    for c in json.loads(path.read_text(encoding="utf-8")):
        name = c.get("name")
        c["name"] = {LANG_EN: name} if isinstance(name, str) else dict(name or {})
        c["aliases"] = list(c.get("aliases") or [])
        out.append(c)
    return out


def add_traditional(names: Names, s2t: Callable[[str], str]) -> bool:
    """Give names without a Traditional Chinese name one converted from the Simplified one."""
    if names.get(LANG_ZH) and not names.get(LANG_TW):
        names[LANG_TW] = s2t(names[LANG_ZH])
        return True
    return False


def build_index(series_cfg: list[dict], data_dir: Path, s2t: Callable[[str], str]) -> dict:
    idx: dict = {"version": 1, "series": [], "chars": []}
    pos: dict[str, int] = {}                      # lowercase English name -> index in idx["chars"]

    for gi, sc in enumerate(series_cfg):
        game = sc["game"]
        path = data_dir / f"{game}.json"
        if not path.exists():
            print(f"no data for {game}, skipping")
            continue
        chars = load_chars(path)
        before = len(chars)
        chars = merge_alters(chars)
        print(f"{game}: merged {before - len(chars)} alter versions into base characters")

        s_idx = len(idx["series"])
        title = {l: t.strip() for l, t in (sc.get("title") or {}).items() if t and t.strip()}
        title.setdefault(LANG_EN, game)           # folder names stay the game names
        idx["series"].append({"title": title, "aliases": union(sc.get("aliases") or [])})

        bgm = bool(sc.get("bangumi_id"))
        base = max(100 - gi * 10, 10)
        n = len(chars)
        for i, c in enumerate(chars):
            # Data files are newest-first, so a higher index means an older character; older
            # characters score higher (base down to base/2).
            score = base - (base // 2) * (n - 1 - i) // n
            if bgm:
                if c.get("collab"):
                    continue
                score = int(c.get("score") or 0)  # 0 when Bangumi has no such character
            key = english(c["name"]).lower()
            if not key:
                continue
            month, day = int(c.get("birthMonth") or 0), int(c.get("birthDay") or 0)
            if key in pos:
                # Same character in another game: keep the first names, add the other games'
                # different names and aliases as aliases.
                out = idx["chars"][pos[key]]
                out["series"].append(s_idx)
                out["score"] = max(out["score"], score)
                if not out.get("birthMonth") and month:
                    out["birthMonth"], out["birthDay"] = month, day
                out["name"], out["aliases"] = fold_names(out["name"], out["aliases"], c)
                continue
            pos[key] = len(idx["chars"])
            out = {"name": dict(c["name"]), "series": [s_idx], "score": score, "aliases": union(c["aliases"])}
            if month:
                out["birthMonth"] = month
            if day:
                out["birthDay"] = day
            idx["chars"].append(out)

    idx["chars"].sort(key=lambda c: -c["score"])  # stable: ties keep their order

    converted = 0
    for c in idx["chars"]:
        if add_traditional(c["name"], s2t):
            converted += 1
            c["aliases"] = without_names(c["aliases"], c["name"])
    titles = sum(add_traditional(s["title"], s2t) for s in idx["series"])
    print(f"Traditional Chinese added by OpenCC: {converted} characters, {titles} series titles")

    for c in idx["chars"]:                        # same key order as the other keys, like KKAFIO's
        c["name"] = dict(sorted(c["name"].items()))
    for s in idx["series"]:
        s["title"] = dict(sorted(s["title"].items()))
    return idx


def make_s2t() -> Callable[[str], str]:
    try:
        from opencc import OpenCC
    except ImportError:
        sys.exit("OpenCC is not installed: pip install OpenCC")
    return OpenCC(OPENCC_CONFIG).convert


def main() -> None:
    root = Path(__file__).resolve().parent.parent
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--out", default="dist/gacha_character_index.json", type=Path, help="output path")
    ap.add_argument("--config", default=root / "config" / "series.json", type=Path)
    ap.add_argument("--data-dir", default=root / "data", type=Path)
    args = ap.parse_args()

    cfg = json.loads(args.config.read_text(encoding="utf-8"))
    idx = build_index(cfg, args.data_dir, make_s2t())
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(idx, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {args.out}: {len(idx['series'])} series, {len(idx['chars'])} characters")


if __name__ == "__main__":
    main()
