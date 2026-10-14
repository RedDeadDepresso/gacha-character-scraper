package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

const (
	fgoRepo   = "atlasacademy/fgo-game-data-api"
	fgoBranch = "master"
	fgoNames  = "https://raw.githubusercontent.com/atlasacademy/fgo-game-data-api/refs/heads/master/app/data/mappings/servant_names.json"
)

// FateGrandOrder reads servants from the Atlas Academy game data API repo. The
// name has no slash so it is safe as a file name.
type FateGrandOrder struct{}

func (FateGrandOrder) Name() string { return "Fate Grand Order" }

type fgoEntry struct {
	jp, en string
}

// Scrape skips everything when the fgo-game-data-api commit hasn't changed
// since the last run (cached in config/commits.json). servant_names.json maps
// Japanese names to English names, oldest servant first; the result is stored
// newest-first with the Japanese name as an alias.
func (f FateGrandOrder) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(fgoRepo, fgoBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(f.Name()) {
		log.Printf("[Fate Grand Order] fgo-game-data-api unchanged (%s), skipping", sha[:7])
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
		oldest[p].Aliases = cleanAliases(oldest[p].Name, append(oldest[p].Aliases, aliases...))
	}
	slot := make([]int, len(entries)) // entry index -> index in oldest
	for i, e := range entries {
		if root[i] != i {
			continue
		}
		key := strings.ToLower(e.en)
		if p, ok := pos[key]; ok {
			slot[i] = p
			add(p, e.jp)
			continue
		}
		pos[key] = len(oldest)
		slot[i] = len(oldest)
		oldest = append(oldest, Character{Name: e.en, Aliases: cleanAliases(e.en, []string{e.jp})})
	}
	for i, e := range entries {
		if root[i] != i {
			add(slot[root[i]], e.jp, e.en)
		}
	}

	// Reverse to newest-first.
	chars := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}

	if err := SaveCommit(f.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Fate Grand Order] %d servants (commit %s)", len(chars), sha[:7])
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
