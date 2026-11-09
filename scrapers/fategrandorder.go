package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
)

const (
	fgoNames = "https://raw.githubusercontent.com/atlasacademy/fgo-game-data-api/refs/heads/master/app/data/mappings/servant_names.json"

	// fgoNice is Atlas Academy's servant export, one file per server. The API
	// sends no ETag for it, so it is not part of the source tag.
	fgoNice = "https://api.atlasacademy.io/export/%s/nice_servant.json"
)

// fgoServers are the servers whose nice_servant.json is merged by servant id,
// with the language of the names each one has.
var fgoServers = []string{"JP", "NA", "CN", "TW", "KR"}

var fgoLang = map[string]string{"JP": LangJA, "NA": LangEN, "CN": LangZH, "TW": LangTW, "KR": LangKO}

// FateGrandOrder reads servants from the Atlas Academy game data API repo. The
// name has no slash so it is safe as a file name.
type FateGrandOrder struct{}

func (FateGrandOrder) Name() string { return "Fate Grand Order" }

type fgoEntry struct {
	jp, en string
}

// Scrape skips everything when servant_names.json's ETag hasn't changed since
// the last run (cached in config/etags.json); the Atlas Academy API sends no
// ETag, so only the repo file is watched. servant_names.json maps Japanese
// names to English names, oldest servant first. Servants from each server's
// nice_servant.json are merged by id and get their names in that server's
// language; the English name from servant_names.json fills in for servants
// that are not in NA, and entries of the file with no matching servant are
// kept as they are. The result is stored newest-first.
func (f FateGrandOrder) Scrape(existing []Character) ([]Character, error) {
	tag, err := SourceTag(fgoNames)
	if err != nil {
		return nil, err
	}
	if upToDate(f.Name(), tag, existing) {
		log.Printf("[Fate Grand Order] servant_names.json unchanged (%s), skipping", tag[:7])
		return existing, nil
	}

	body, err := get(fgoNames)
	if err != nil {
		return nil, err
	}
	entries, err := readFGONames(body)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse servant_names.json: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("fate grand order: no servants parsed")
	}

	// The file also lists short forms right next to the full name ("Mash" beside
	// "Mash Kyrielight"). When an entry's Japanese and English names both start
	// the neighbouring entry's, fold it into that neighbour as an alias. Only
	// neighbours are considered so unrelated servants that merely share a first
	// name (Ishtar / Ishtar Ashtart) stay separate.
	root := make([]int, len(entries))
	for i, e := range entries {
		root[i] = i
		for _, j := range []int{i - 1, i + 1} {
			if j >= 0 && j < len(entries) && isFGOShortForm(e, entries[j]) {
				root[i] = j
				break
			}
		}
	}
	for i := range root { // follow chains to the final parent
		for root[i] != root[root[i]] {
			root[i] = root[root[i]]
		}
	}

	// Oldest-first pass; entries sharing an English name merge into the oldest.
	var oldest []Character
	pos := map[string]int{}
	add := func(p int, aliases ...string) {
		oldest[p] = builderOf(oldest[p]).Alias(aliases...).Character()
	}
	slot := make([]int, len(entries)) // entry index -> index in oldest
	for i, e := range entries {
		if root[i] != i {
			continue
		}
		key := strings.ToLower(e.en)
		if p, ok := pos[key]; ok {
			slot[i] = p
			oldest[p] = builderOf(oldest[p]).Name(LangJA, e.jp).Character()
			continue
		}
		pos[key] = len(oldest)
		slot[i] = len(oldest)
		oldest = append(oldest, newNameBuilder().Name(LangEN, e.en).Name(LangJA, e.jp).Character())
	}
	for i, e := range entries {
		if root[i] != i {
			add(slot[root[i]], e.jp, e.en)
		}
	}

	// Reverse to newest-first.
	fromNames := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		fromNames = append(fromNames, oldest[i])
	}

	servants := map[int]*fgoServant{}
	for _, server := range fgoServers {
		list, err := fetchFGOServants(server)
		if err != nil {
			return nil, err
		}
		mergeFGOServants(servants, server, list)
	}
	chars := mergeFGONames(fromNames, servants)
	if len(chars) == 0 {
		return nil, fmt.Errorf("fate grand order: no servants found")
	}

	if err := SaveTag(f.Name(), tag); err != nil {
		return nil, err
	}
	log.Printf("[Fate Grand Order] %d servants, %d from nice_servant (etag %s)", len(chars), len(servants), tag[:7])
	return chars, nil
}

// isFGOShortForm reports whether s is a shortened form of full: its Japanese
// name starts the other's, and its English name starts the other's at a word
// boundary. Variants ("X (Alter)", "X Alter") are separate characters, not
// short forms of the base.
func isFGOShortForm(s, full fgoEntry) bool {
	if len(s.jp) >= len(full.jp) || !strings.HasPrefix(full.jp, s.jp) ||
		len(s.en) >= len(full.en) || !strings.HasPrefix(full.en, s.en+" ") {
		return false
	}
	rest := strings.TrimPrefix(full.en, s.en+" ")
	return !strings.HasPrefix(rest, "(") && !strings.EqualFold(rest, "Alter")
}

// readFGONames reads the JSON object in file order (a Go map would lose it).
func readFGONames(r interface{ Read([]byte) (int, error) }) ([]fgoEntry, error) {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var out []fgoEntry
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		jp, _ := t.(string)
		var en string
		if err := dec.Decode(&en); err != nil {
			return nil, err
		}
		jp, en = strings.TrimSpace(jp), strings.Join(strings.Fields(en), " ")
		if jp != "" && en != "" {
			out = append(out, fgoEntry{jp: jp, en: en})
		}
	}
	return out, nil
}

// fgoServant is one servant merged across servers by id.
type fgoServant struct {
	id, collectionNo int
	names            map[string]string // server code -> name
}

// fgoNiceEntry is the part of a nice_servant.json entry that is used.
type fgoNiceEntry struct {
	ID           int    `json:"id"`
	CollectionNo int    `json:"collectionNo"`
	Name         string `json:"name"`
}

// fetchFGOServants downloads one server's nice_servant.json. A server whose
// file does not exist (404) yields no servants.
func fetchFGOServants(server string) ([]fgoNiceEntry, error) {
	body, err := get(fmt.Sprintf(fgoNice, server))
	if errors.Is(err, ErrNotFound) {
		log.Printf("[Fate Grand Order] no nice_servant.json for %s, skipping", server)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer body.Close()
	list, err := readFGOServants(body)
	if err != nil {
		return nil, fmt.Errorf("parse nice_servant.json (%s): %w", server, err)
	}
	return list, nil
}

func readFGOServants(r io.Reader) ([]fgoNiceEntry, error) {
	var list []fgoNiceEntry
	if err := json.NewDecoder(r).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// mergeFGOServants adds one server's servants to the map, keyed by id.
func mergeFGOServants(into map[int]*fgoServant, server string, list []fgoNiceEntry) {
	for _, e := range list {
		name := strings.Join(strings.Fields(e.Name), " ")
		if e.ID == 0 || name == "" {
			continue
		}
		s, ok := into[e.ID]
		if !ok {
			s = &fgoServant{id: e.ID, names: map[string]string{}}
			into[e.ID] = s
		}
		if s.collectionNo == 0 {
			s.collectionNo = e.CollectionNo
		}
		s.names[server] = name
	}
}

// mergeFGONames builds the final list, newest first. fromNames are the
// characters built from servant_names.json (newest first, Japanese name in
// ja_jp). Every servant becomes a character with the name of each server in
// its language. Names are never dropped: a servant missing from NA keeps its
// Japanese (and Chinese) names and takes the English one from fromNames when
// its Japanese name matches; a differing English name becomes an alias. Entries of
// fromNames that match no servant are appended unchanged.
func mergeFGONames(fromNames []Character, servants map[int]*fgoServant) []Character {
	byJA := map[string]int{} // ja_jp name -> index in fromNames
	for i, c := range fromNames {
		if ja := c.Name[LangJA]; ja != "" {
			byJA[ja] = i
		}
	}

	list := make([]*fgoServant, 0, len(servants))
	for _, s := range servants {
		list = append(list, s)
	}
	// Newest first: servants in the collection by number, the rest (no
	// collection number) after them, by id.
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.collectionNo == 0) != (b.collectionNo == 0) {
			return b.collectionNo == 0
		}
		if a.collectionNo != b.collectionNo {
			return a.collectionNo > b.collectionNo
		}
		return a.id > b.id
	})

	used := make([]bool, len(fromNames))
	out := make([]Character, 0, len(list)+len(fromNames))
	for _, s := range list {
		b := newNameBuilder()
		// English first so the NA name is the main one; then the others.
		for _, server := range []string{"NA", "JP", "CN", "TW", "KR"} {
			b.Name(fgoLang[server], s.names[server])
		}
		if i, ok := byJA[s.names["JP"]]; ok && !used[i] {
			used[i] = true
			b.Merge(fromNames[i])
		}
		out = append(out, b.Character())
	}
	for i, c := range fromNames {
		if !used[i] {
			out = append(out, c)
		}
	}
	return out
}