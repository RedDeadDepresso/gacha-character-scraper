import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts"))
import merge  # noqa: E402

EN, ZH, TW, JA, KO = merge.LANG_EN, merge.LANG_ZH, merge.LANG_TW, merge.LANG_JA, merge.LANG_KO


def s2t(text: str) -> str:            # stands in for OpenCC: 绫 -> 綾, 华 -> 華
    return text.translate(str.maketrans("绫华奥黛塔", "綾華奧黛塔"))


def ch(name, **kw):
    return {"name": name if isinstance(name, dict) else {EN: name}, "aliases": kw.pop("aliases", []), **kw}


@pytest.mark.parametrize("alter", ["Nemesis: Gnosis", "Nemesis : Gnosis", "Nemesis・Gnosis", "Nemesis·Gnosis", "Nemesis • Gnosis"])
def test_merge_alters_separators(alter):
    nemesis = {EN: "Nemesis", JA: "ネメシス", KO: "네메시스", ZH: "纳美西丝", TW: "納美西絲"}
    got = merge.merge_alters([
        ch({EN: alter, JA: "ネメシス・ グノーシス", KO: "네메시스·연광", ZH: "纳美西丝·衍光", TW: "納美西絲·衍光"}),
        ch(nemesis),
        ch("Krolik"),
    ])
    assert [merge.english(c["name"]) for c in got] == ["Nemesis", "Krolik"]
    for want in (alter, "ネメシス・ グノーシス", "네메시스·연광", "纳美西丝·衍光", "納美西絲·衍光"):
        assert want in got[0]["aliases"]
    assert got[0]["name"][JA] == "ネメシス"


def test_merge_alters_without_a_base_stay_separate():
    got = merge.merge_alters([ch("Re:Zero"), ch("Rem")])
    assert [(c["name"], c["aliases"]) for c in got] == [({EN: "Re:Zero"}, []), ({EN: "Rem"}, [])]


def test_merge_alters_keep_the_base_score_collab_and_birthday():
    got = merge.merge_alters([
        ch("Amiya (Guardian)", score=5),
        ch("Amiya", score=900, collab=True, birthMonth=12, birthDay=23),
    ])
    assert len(got) == 1 and (got[0]["score"], got[0]["collab"], got[0]["birthMonth"], got[0]["birthDay"]) == (900, True, 12, 23)


def test_merge_alters_chain_and_alias_cleanup():
    got = merge.merge_alters([ch("Saber (Alter) the Dark"), ch("Saber (Alter)"), ch("Saber", aliases=["saber", "Artoria"])])
    assert len(got) == 1 and got[0]["aliases"] == ["Artoria", "Saber (Alter) the Dark", "Saber (Alter)"]


def write(tmp_path, game, chars):
    (tmp_path / f"{game}.json").write_text(json.dumps(chars, ensure_ascii=False), encoding="utf-8")


def test_build_index(tmp_path):
    write(tmp_path, "Alpha", [   # newest first
        ch({EN: "Newest", ZH: "奥黛塔"}, score=7), ch("Collab", score=999, collab=True),
        ch({EN: "Ayaka", JA: "綾華", ZH: "绫华"}, score=900, birthMonth=9, birthDay=28),
        ch("Shared", score=5, aliases=["S1"]),
    ])
    write(tmp_path, "Beta", [ch("Oldest", score=1), ch({EN: "Shared", JA: "共通"}, aliases=["S2"]), ch("Nobody")])
    write(tmp_path, "Plain", [ch("P-new"), ch("P-old")])
    cfg = [
        {"game": "Alpha", "bangumi_id": 1, "title": {EN: "Alpha Game", ZH: "阿尔法 华", " ": " "}, "aliases": ["A"]},
        {"game": "Missing", "bangumi_id": 2},
        {"game": "Beta", "bangumi_id": 3, "title": {}},
        {"game": "Plain", "title": {EN: "Plain"}},      # no bangumi_id: scored by position
    ]
    idx = merge.build_index(cfg, tmp_path, s2t)

    assert idx["version"] == 1
    assert [s["title"] for s in idx["series"]] == [
        {EN: "Alpha Game", ZH: "阿尔法 华", TW: "阿尔法 華"}, {EN: "Beta"}, {EN: "Plain"}]   # "Missing" has no series
    assert idx["series"][0]["aliases"] == ["A"]
    by = {c["name"][EN]: c for c in idx["chars"]}
    assert "Collab" not in by                                           # collab characters are left out
    assert by["Ayaka"]["score"] == 900 and (by["Ayaka"]["birthMonth"], by["Ayaka"]["birthDay"]) == (9, 28)
    assert "birthMonth" not in by["Newest"]
    assert by["Nobody"]["score"] == 0 and by["Newest"]["score"] == 7    # Bangumi score as it is
    shared = by["Shared"]
    assert shared["series"] == [0, 1] and shared["score"] == 5
    assert shared["name"][JA] == "共通" and sorted(shared["aliases"]) == ["S1", "S2"]
    # no bangumi_id: base 100 - 3*10 = 70 down to 70/2 over the list, the older one higher
    assert by["P-old"]["score"] == 70 and by["P-new"]["score"] == 70 - 35 * 1 // 2
    scores = [c["score"] for c in idx["chars"]]
    assert scores == sorted(scores, reverse=True)


def test_traditional_chinese_is_added_only_where_missing(tmp_path):
    write(tmp_path, "G", [
        ch({EN: "Odette", ZH: "奥黛塔"}),                                  # no zh_tw: converted
        ch({EN: "Keep", ZH: "绫华", TW: "手寫"}),                          # has one: kept
        ch({EN: "NoZh", JA: "華"}),                                        # no zh_cn: nothing to convert
        ch({EN: "Alias", ZH: "绫华"}, aliases=["綾華", "other"]),           # alias equal to the new name goes
    ])
    idx = merge.build_index([{"game": "G", "title": {EN: "G", ZH: "华"}}], tmp_path, s2t)
    by = {c["name"][EN]: c for c in idx["chars"]}
    assert by["Odette"]["name"][TW] == "奧黛塔"
    assert by["Keep"]["name"][TW] == "手寫"
    assert TW not in by["NoZh"]["name"]
    assert by["Alias"]["name"][TW] == "綾華" and by["Alias"]["aliases"] == ["other"]
    assert idx["series"][0]["title"][TW] == "華"
    assert list(by["Odette"]["name"]) == sorted(by["Odette"]["name"])  # keys sorted


def test_old_single_string_names_load(tmp_path):
    (tmp_path / "G.json").write_text('[{"name": "Old", "aliases": null}]', encoding="utf-8")
    idx = merge.build_index([{"game": "G"}], tmp_path, s2t)
    assert idx["chars"][0]["name"] == {EN: "Old"} and idx["chars"][0]["aliases"] == []


def test_opencc_converts_with_taiwan_glyphs():
    pytest.importorskip("opencc")
    assert merge.make_s2t()("奥黛塔 鸣潮") == "奧黛塔 鳴潮"
    assert merge.make_s2t()("莉莉丝·里德") == "莉莉絲·裡德"      # s2t would give 裏德


def test_main_writes_the_file(tmp_path, monkeypatch):
    write(tmp_path, "G", [ch({EN: "Odette", ZH: "奥黛塔"})])
    (tmp_path / "series.json").write_text('[{"game": "G", "title": {"en_us": "G"}}]', encoding="utf-8")
    out = tmp_path / "dist" / "i.json"
    monkeypatch.setattr(merge, "make_s2t", lambda: s2t)
    monkeypatch.setattr(sys, "argv", ["merge.py", "--out", str(out), "--config", str(tmp_path / "series.json"), "--data-dir", str(tmp_path)])
    merge.main()
    text = out.read_text(encoding="utf-8")
    assert text.endswith("\n") and "奧黛塔" in text and json.loads(text)["chars"][0]["name"][TW] == "奧黛塔"
