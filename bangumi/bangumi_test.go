package bangumi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func jl(recs ...any) string {
	var sb strings.Builder
	for _, r := range recs {
		b, _ := json.Marshal(r)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

type m = map[string]any

// Game 100. Characters: 1 Ayaka (main, linked twice: guest and main), 2 Aloy (a
// guest; she is a main character of subject 50), 3 Paimon (supporting, a guest in
// an anime), 4 Nobody (broken infobox, guest as "3"), 5 is in another game.
func writeDump(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		SubjectsFile: jl(
			m{"id": 100, "name": "原神", "name_cn": "原神", "type": 4, "date": "2020-09-28", "summary": "x"},
			m{"id": 50, "name": "Horizon", "date": "2017-02-28"},
			m{"id": 101, "name": "Genshin anime", "date": "2022-01-01"},
			m{"id": "200", "name": "Other game", "date": ""},
		),
		LinksFile: jl(
			m{"subject_id": 100, "character_id": 1, "type": 3}, m{"subject_id": 100, "character_id": 2, "type": 3},
			m{"subject_id": 50, "character_id": 2, "type": 1}, m{"subject_id": 100, "character_id": 3, "type": 2},
			m{"subject_id": 101, "character_id": 3, "type": 3}, m{"subject_id": 100, "character_id": 4, "type": "3"},
			m{"subject_id": 200, "character_id": 5, "type": 1}, m{"subject_id": 100, "character_id": 1, "type": 1},
		),
		CharactersFile: jl(
			m{"id": 1, "name": "神里 綾華", "collects": 900, "infobox": "{{Infobox Crt\n|简体中文名= 神里绫华\n|性别= 女\n|别名={\n[英文名|Kamisato Ayaka]\n[罗马字|Kamisato Ayaka]\n}\n}}"},
			m{"id": 2, "name": "アーロイ", "collects": 50, "infobox": "{{Infobox Crt\n|别名={\n[英文名|Aloy]\n}\n}}"},
			m{"id": 3, "name": "パイモン", "collects": "700", "infobox": "{{Infobox Crt\n|简体中文名= 派蒙\n}}"},
			m{"id": 4, "name": "Nobody", "collects": 1, "infobox": "broken"},
			m{"id": 5, "name": "神里 綾華", "collects": 99999, "infobox": ""},
		),
	}
	for n, s := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNormAndForms(t *testing.T) {
	if Norm("Ｃｈ’en: The Dawn-streak ・ 1") != "chenthedawnstreak1" {
		t.Errorf("Norm: %q", Norm("Ｃｈ’en: The Dawn-streak ・ 1"))
	}
	got := Forms("近藤奈々 (こんどう なな)")
	sort.Strings(got)
	for _, w := range []string{"近藤奈々こんどうなな", "近藤奈々", "こんどうなな"} {
		if i := sort.SearchStrings(got, w); i >= len(got) || got[i] != w {
			t.Errorf("Forms missing %q in %v", w, got)
		}
	}
	for _, w := range [][2]string{{"Sunaookami, Shiroko", "Shiroko Sunaookami"}, {"鲁路修·兰佩路基", "兰佩路基鲁路修"}} {
		if !contains(Forms(w[0]), Norm(w[1])) {
			t.Errorf("%q should be found as %q", w[0], w[1])
		}
	}
	if len(Forms("A")) != 0 {
		t.Error("one-letter forms must be dropped")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestLoadAndMatch(t *testing.T) {
	dir := t.TempDir()
	writeDump(t, dir)
	d, err := Load(dir, []int{100, 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(d.Characters(100)); got != 4 {
		t.Fatalf("characters of 100: %d (5 is in another game, 1 is linked twice)", got)
	}
	if c := d.chars[3]; c.Collects != 700 || d.IsCollab(100, c) {
		t.Errorf("Paimon: %+v (a guest role in the anime says nothing about the game)", c)
	}
	if d.IsCollab(100, d.chars[1]) {
		t.Error("Ayaka is also linked as a main character")
	}
	if !d.IsCollab(100, d.chars[2]) || !d.IsCollab(100, d.chars[4]) {
		t.Error("Aloy (type 3) and Nobody (type \"3\") are collab characters")
	}
	if n, typ, ok := d.Subject(100); !ok || n != "原神" || typ != 4 {
		t.Errorf("Subject(100) = %q %d %v", n, typ, ok)
	}
	if _, _, ok := d.Subject(12345); ok {
		t.Error("unknown subject")
	}
	if contains(d.chars[1].Names, "女") {
		t.Errorf("non-name infobox fields must not be names: %v", d.chars[1].Names)
	}

	res := d.Match(100, []Scraped{
		{Names: []string{"Kamisato Ayaka", "神里绫华"}},                    // 0: by name
		{Names: []string{"Aloy"}},                                      // 1: collab
		{Names: []string{"Not it"}, Aliases: []string{"Paimon", "派蒙"}}, // 2: by alias only
		{Names: []string{"Brand New"}},                                 // 3: not in Bangumi
		{Names: []string{"Nobody"}},                                    // 4: broken infobox still matches by name (guest: collab)
	})
	want := []Result{
		{Matched: true, Collects: 900},
		{Matched: true, Collects: 50, Collab: true},
		{Matched: true, Collects: 700},
		{},
		{Matched: true, Collects: 1, Collab: true},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("got  %+v\nwant %+v", res, want)
	}
}

func TestMatchPrefersTheBestCandidate(t *testing.T) {
	d := &Data{bySubject: map[int][]int{1: {1, 2}}, chars: map[int]*Character{
		1: {ID: 1, Collects: 10, Names: []string{"Mizuki", "瑞希"}},
		2: {ID: 2, Collects: 99, Names: []string{"Mizuki Akiyama", "Mizuki"}},
	}}
	// Both share "Mizuki"; only 2 shares the full name too.
	if r := d.Match(1, []Scraped{{Names: []string{"Mizuki Akiyama", "Mizuki"}}}); r[0].Collects != 99 {
		t.Errorf("%+v", r)
	}
	// Same score: the more popular one.
	if r := d.Match(1, []Scraped{{Names: []string{"Mizuki"}}}); r[0].Collects != 99 {
		t.Errorf("%+v", r)
	}
}

// --- Update ---------------------------------------------------------------

func fakeZip(t *testing.T) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"dump/subject.jsonlines": "s\n", "dump/subject-characters.jsonlines": "l\n",
		"dump/character.jsonlines": "c\n", "dump/person.jsonlines": "never extracted\n",
	} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	return buf.Bytes()
}

func release(url string, body []byte, bad bool) []byte {
	sum := sha256.Sum256(body)
	d := hex.EncodeToString(sum[:])
	if bad {
		d = strings.Repeat("0", 64)
	}
	b, _ := json.Marshal(m{"name": "dump.zip", "browser_download_url": url + "/dump.zip", "digest": "sha256:" + d, "size": len(body)})
	return b
}

func TestUpdate(t *testing.T) {
	body := fakeZip(t)
	var latest []byte
	var downloads int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest.json" {
			w.Write(latest)
			return
		}
		downloads++
		w.Write(body)
	}))
	defer srv.Close()
	old := latestURL
	latestURL = srv.URL + "/latest.json"
	defer func() { latestURL = old }()
	dir := filepath.Join(t.TempDir(), "dump")

	latest = release(srv.URL, body, true)
	if err := Update(dir); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want a sha256 error, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a failed download must leave nothing behind: %v", entries)
	}

	latest = release(srv.URL, body, false)
	if err := Update(dir); err != nil {
		t.Fatal(err)
	}
	var names []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if want := fmt.Sprint([]string{"character.jsonlines", "latest.json", "subject-characters.jsonlines", "subject.jsonlines"}); fmt.Sprint(names) != want {
		t.Errorf("dir = %v, want only the needed files + latest.json", names)
	}

	before := downloads
	if err := Update(dir); err != nil || downloads != before {
		t.Errorf("unchanged latest.json must not download again (err %v)", err)
	}
	os.Remove(filepath.Join(dir, CharactersFile))
	if err := Update(dir); err != nil || downloads != before+1 {
		t.Errorf("a missing file must be downloaded again (err %v)", err)
	}

	srv.Close() // offline: the existing dump is used
	if err := Update(dir); err != nil {
		t.Errorf("offline with a complete dump: %v", err)
	}
	if err := Update(filepath.Join(t.TempDir(), "empty")); err == nil {
		t.Error("offline without a dump must fail")
	}
}
