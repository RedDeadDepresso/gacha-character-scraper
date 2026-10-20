package bangumi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wiki "github.com/bangumi/wiki-parser-go"
)

// flexInt reads a JSON number or a numeric string; anything else is 0 (the dump
// is not strict about types).
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
			v = int(f)
		} else {
			v = 0
		}
	}
	*n = flexInt(v)
	return nil
}

// readJSONL calls fn with every non-empty line of a .jsonlines file. Lines can
// be very long (summaries, comments), so no fixed-size scanner buffer is used.
func readJSONL(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			fn(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type subjectRec struct {
	ID     flexInt `json:"id"`
	Name   string  `json:"name"`
	NameCN string  `json:"name_cn"`
	Date   string  `json:"date"`
	Type   int     `json:"type"`
}

type linkRec struct {
	SubjectID   flexInt `json:"subject_id"`
	CharacterID flexInt `json:"character_id"`
	Type        flexInt `json:"type"` // role in the subject: 1 main, 2 supporting, 3 guest (collab)
}

type characterRec struct {
	ID       flexInt `json:"id"`
	Name     string  `json:"name"`
	Infobox  string  `json:"infobox"`
	Collects flexInt `json:"collects"`
}

// Character is a Bangumi character that appears in one of the loaded subjects.
type Character struct {
	ID       int
	Collects int      // 收藏数: the popularity
	Names    []string // the name, the simplified Chinese name and every alias

	// BirthMonth and BirthDay come from the infobox `生日` (0 when it has none
	// or no month and day can be read from it).
	BirthMonth, BirthDay int
}

// Data holds the characters of the subjects given to Load.
type Data struct {
	chars     map[int]*Character
	bySubject map[int][]int      // subject id -> character ids (in dump order)
	guest     map[[2]int]bool    // {subject id, character id} -> only linked as a guest (`type` 3)
	subjects  map[int]subjectRec // the subjects given to Load
}

// Subject returns the name and Bangumi type (2 anime, 4 game, ...) of a subject
// given to Load, to check that a bangumi_id is the game it should be.
func (d *Data) Subject(id int) (name string, typ int, ok bool) {
	s, ok := d.subjects[id]
	return firstNonEmpty(s.NameCN, s.Name), s.Type, ok
}

// Load reads the dump in dir (see Update) for the given subjects: the
// characters linked to them, their role (`type`) in each and their names.
func Load(dir string, subjectIDs []int) (*Data, error) {
	want := map[int]bool{}
	for _, id := range subjectIDs {
		if id != 0 {
			want[id] = true
		}
	}
	d := &Data{
		chars: map[int]*Character{}, bySubject: map[int][]int{},
		guest: map[[2]int]bool{}, subjects: map[int]subjectRec{},
	}
	if len(want) == 0 {
		return d, nil
	}

	// 1. The characters of the subjects and their role in them.
	inSubjects := map[int]bool{}
	err := readJSONL(filepath.Join(dir, LinksFile), func(line []byte) {
		var l linkRec
		if json.Unmarshal(line, &l) != nil || !want[int(l.SubjectID)] || l.CharacterID == 0 {
			return
		}
		sid, cid := int(l.SubjectID), int(l.CharacterID)
		key := [2]int{sid, cid}
		g := int(l.Type) == guestType
		if wasGuest, seen := d.guest[key]; !seen {
			d.bySubject[sid] = append(d.bySubject[sid], cid)
			d.guest[key] = g
		} else if wasGuest && !g { // a repeated link as main / supporting wins
			d.guest[key] = false
		}
		inSubjects[cid] = true
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", LinksFile, err)
	}

	// 2. The subjects themselves (only their name, to explain a wrong id).
	err = readJSONL(filepath.Join(dir, SubjectsFile), func(line []byte) {
		var s subjectRec
		if json.Unmarshal(line, &s) != nil || !want[int(s.ID)] {
			return
		}
		d.subjects[int(s.ID)] = s
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", SubjectsFile, err)
	}

	// 3. The characters.
	err = readJSONL(filepath.Join(dir, CharactersFile), func(line []byte) {
		var r characterRec
		if json.Unmarshal(line, &r) != nil || !inSubjects[int(r.ID)] {
			return
		}
		names, birthday := parseInfobox(r)
		c := &Character{ID: int(r.ID), Collects: int(r.Collects), Names: names}
		c.BirthMonth, c.BirthDay = ParseBirthday(birthday)
		d.chars[int(r.ID)] = c
	})
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", CharactersFile, err)
	}
	return d, nil
}

// guestType is the `type` of a subject-characters link for a guest (客串)
// character; 1 is a main and 2 a supporting character.
const guestType = 3

// IsCollab reports whether a character is a collab character of a subject:
// it is linked to the game's subject as a guest (`type` 3) and not also as a
// main or supporting character.
func (d *Data) IsCollab(subjectID int, c *Character) bool {
	return d.guest[[2]int{subjectID, c.ID}]
}

// Characters returns the characters linked to a subject, in dump order.
func (d *Data) Characters(subjectID int) []*Character {
	var out []*Character
	for _, id := range d.bySubject[subjectID] {
		if c := d.chars[id]; c != nil {
			out = append(out, c)
		}
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// parseInfobox collects the names of a character: the name, the Simplified
// Chinese name (`简体中文名`) and every item of the aliases (`别名`: other Chinese
// names, English / Japanese names, romaji, nicknames) of the wiki infobox, and
// returns the text of its `生日` (birthday). Other infobox fields (gender, ...)
// are not names and are skipped.
func parseInfobox(r characterRec) (names []string, birthday string) {
	names = []string{r.Name}
	w, err := wiki.Parse(r.Infobox)
	if err != nil { // a broken infobox still has the plain name
		return clean(names), ""
	}
	for _, f := range w.Fields {
		switch {
		case f.Key == "简体中文名" && !f.Array:
			names = append(names, f.Value)
		case f.Key == "别名" && f.Array:
			for _, it := range f.Values {
				names = append(names, it.Value)
			}
		case f.Key == "别名" && f.Value != "": // written as a plain value
			names = append(names, f.Value)
		case f.Key == "生日" && !f.Array:
			birthday = f.Value
		case f.Key == "生日" && len(f.Values) > 0: // written as a list: the first item
			birthday = f.Values[0].Value
		}
	}
	return clean(names), birthday
}

func clean(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
