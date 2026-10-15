package scrapers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLangCode(t *testing.T) {
	for in, want := range map[string]string{
		"en": LangEN, "jp": LangJA, "ja_JP": LangJA, "kr": LangKO, "ko-KR": LangKO,
		"zh-Hans": LangZH, "chs": LangZH, "cn": LangZH, "zh_CN": LangZH,
		"zh-Hant": LangTW, "cht": LangTW, "tw": LangTW, "zh-TW": LangTW, "ru_RU": LangRU, "fr": "",
	} {
		if got := LangCode(in); got != want {
			t.Errorf("LangCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuilder(t *testing.T) {
	c := newNameBuilder().
		Name(LangEN, " Kamisato   Ayaka ").
		Name("ja", "神里綾華").
		Name("jp", "神里 綾華").          // second Japanese name -> alias
		Name("ko", "Kamisato Ayaka"). // same as English -> dropped
		Name("xx", "ignored").
		Alias("Ayaka", "神里綾華", "").
		Character()
	want := Character{
		Name:    Names{LangEN: "Kamisato Ayaka", LangJA: "神里綾華"},
		Aliases: []string{"神里 綾華", "Ayaka"},
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v, want %+v", c, want)
	}
}

func TestBuilderNoEnglish(t *testing.T) {
	c := newNameBuilder().Name(LangJA, "ルーラーシップ").Character()
	if c.Name[LangEN] != "ルーラーシップ" {
		t.Errorf("en_us not promoted: %+v", c.Name)
	}
}

func TestMerge(t *testing.T) {
	a := newNameBuilder().Name(LangEN, "Red").Name(LangJA, "レッド").Character()
	b := newNameBuilder().Name(LangEN, "Red").Name(LangJA, "レッド（別）").Name(LangKO, "레드").Alias("R").Character()
	m := builderOf(a).Merge(b).Character()
	want := Character{
		Name:    Names{LangEN: "Red", LangJA: "レッド", LangKO: "레드"},
		Aliases: []string{"レッド（別）", "R"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %+v, want %+v", m, want)
	}
}

func TestCharacterJSON(t *testing.T) {
	var cs []Character
	old := `[{"name":"Roxy","aliases":["洛克茜"]}]`
	if err := json.Unmarshal([]byte(old), &cs); err != nil {
		t.Fatal(err)
	}
	if !cs[0].legacy || cs[0].Name[LangEN] != "Roxy" {
		t.Errorf("legacy not read: %+v", cs[0])
	}
	cur := `[{"name":{"en_us":"Roxy","zh_cn":"洛克茜"},"aliases":[]}]`
	cs = nil
	if err := json.Unmarshal([]byte(cur), &cs); err != nil {
		t.Fatal(err)
	}
	if cs[0].legacy || cs[0].Name[LangZH] != "洛克茜" {
		t.Errorf("new format misread: %+v", cs[0])
	}
	b, _ := json.Marshal(cs[0])
	if string(b) != `{"name":{"en_us":"Roxy","zh_cn":"洛克茜"},"aliases":[]}` {
		t.Errorf("marshal: %s", b)
	}
}

func TestBuildArknights(t *testing.T) {
	en := []arknightsOp{{ID: "char_100_a", Name: "Ch'en", CN: "陈"}, {ID: "char_200_b", Name: "Veen", CN: "维恩"}}
	others := map[string][]arknightsOp{
		"jp": {{ID: "char_100_a", Name: "チェン"}},
		"kr": {{ID: "char_100_a", Name: "첸"}},
		"cn": {{ID: "char_100_a", Name: "陈"}},
	}
	got := buildArknightsCharacters(en, others)
	if len(got) != 2 || got[0].Name[LangEN] != "Veen" || got[0].Name[LangZH] != "维恩" {
		t.Fatalf("unexpected: %+v", got)
	}
	want := Names{LangEN: "Ch'en", LangJA: "チェン", LangKO: "첸", LangZH: "陈"}
	if !reflect.DeepEqual(got[1].Name, want) {
		t.Errorf("got %+v, want %+v", got[1].Name, want)
	}
}

func TestBuildUma(t *testing.T) {
	s := func(v string) *string { return &v }
	got := buildUmaCharacters([]umaCard{
		{CharID: 1, NameEN: s("Special Week"), NameJP: s("スペシャルウィーク"), NameKO: s("스페셜 위크")},
		{CharID: 1, NameTW: s("特別週")},
	})
	want := Names{LangEN: "Special Week", LangJA: "スペシャルウィーク", LangKO: "스페셜 위크", LangTW: "特別週"}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Name, want) {
		t.Errorf("got %+v", got)
	}
}

func TestBlueArchiveNames(t *testing.T) {
	en := schaleStudent{FamilyName: "Saten", PersonalName: "Ruiko", Name: "Ruiko"}
	jp := schaleStudent{FamilyName: "佐天", PersonalName: "涙子", Name: "涙子"}
	if got := displayName(jp); got != "佐天, 涙子" {
		t.Errorf("displayName = %q", got)
	}
	if got := langAliases("en", en); !reflect.DeepEqual(got, []string{"Ruiko Saten", "Ruiko"}) {
		t.Errorf("en aliases = %v", got)
	}
	if got := langAliases("jp", jp); !reflect.DeepEqual(got, []string{"佐天涙子", "佐天 涙子", "涙子"}) {
		t.Errorf("jp aliases = %v", got)
	}
}
