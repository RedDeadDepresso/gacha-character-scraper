package scrapers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/reddeaddepresso/gacha-character-scraper/bangumi"
)

func TestApplyBangumi(t *testing.T) {
	dir := t.TempDir()
	for n, body := range map[string]string{
		bangumi.SubjectsFile: `{"id":100,"name":"Game","date":"2020-01-01"}
{"id":300,"name":"Older game","date":"2010-01-01"}
`,
		bangumi.LinksFile: `{"subject_id":100,"character_id":1,"type":1}
{"subject_id":100,"character_id":2,"type":3}
{"subject_id":300,"character_id":2,"type":1}
`,
		bangumi.CharactersFile: `{"id":1,"name":"Ayaka","collects":900,"infobox":""}
{"id":2,"name":"Aloy","collects":50,"infobox":""}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bg, err := bangumi.Load(dir, []int{100})
	if err != nil {
		t.Fatal(err)
	}
	chars := []Character{
		{Name: Names{LangEN: "Ayaka"}}, {Name: Names{LangEN: "Aloy"}}, {Name: Names{LangEN: "New"}, Score: 7, Collab: true},
	}
	ApplyBangumi(bg, Series{Game: "Game", BangumiID: 100}, chars)
	if chars[0].Score != 900 || chars[0].Collab ||
		chars[1].Score != 50 || !chars[1].Collab ||
		chars[2].Score != 0 || chars[2].Collab {
		t.Errorf("%+v", chars)
	}

	// No bangumi_id (or no data): untouched.
	other := []Character{{Name: Names{LangEN: "Ayaka"}, Score: 3}}
	ApplyBangumi(bg, Series{Game: "Game"}, other)
	ApplyBangumi(nil, Series{Game: "Game", BangumiID: 100}, other)
	if other[0].Score != 3 {
		t.Errorf("%+v", other)
	}

	// Score and Collab survive a save / load; collab is omitted when false.
	b, _ := json.Marshal(chars[:2])
	var back []Character
	if err := json.Unmarshal(b, &back); err != nil || back[0].Score != 900 || !back[1].Collab {
		t.Errorf("round trip: %v %+v", err, back)
	}
	if string(b) == "" || json.Valid(b) == false || contains(string(b), `"collab":false`) {
		t.Errorf("collab:false must be omitted: %s", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
