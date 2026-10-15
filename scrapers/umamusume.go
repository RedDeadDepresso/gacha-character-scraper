package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

const (
	umaRepo        = "daftuyda/UmaTools"
	umaBranch      = "HEAD" // the repo's default branch
	umaManifestURL = "https://gametora.com/data/manifests/umamusume.json"
	umaDatasetURL  = "https://gametora.com/data/umamusume/%s.%s.json"
	umaDatasetKey  = "character-cards"
)

// UmaMusume reads the playable characters from GameTora's data files.
type UmaMusume struct{}

func (UmaMusume) Name() string { return "Uma Musume Pretty Derby" }

// flexInt decodes a JSON number or a numeric string.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("not an integer: %s", b)
	}
	*f = flexInt(n)
	return nil
}

// umaCard is one entry of GameTora's character-cards dataset: an outfit of a
// character. Every card of a character shares the same char_id and names.
type umaCard struct {
	CharID flexInt `json:"char_id"`
	CardID flexInt `json:"card_id"`
	NameEN *string `json:"name_en"`
	NameJP *string `json:"name_jp"`
	NameKO *string `json:"name_ko"`
	NameTW *string `json:"name_tw"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// fetchUmaCards downloads the manifest, then the character-cards file whose
// name carries the hash listed in it (character-cards.<hash>.json).
func fetchUmaCards() ([]umaCard, error) {
	body, err := get(umaManifestURL)
	if err != nil {
		return nil, err
	}
	var manifest map[string]json.RawMessage
	err = json.NewDecoder(body).Decode(&manifest)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse uma manifest: %w", err)
	}
	hash := strings.Trim(strings.TrimSpace(string(manifest[umaDatasetKey])), `"`)
	if hash == "" || hash == "null" {
		return nil, fmt.Errorf("uma manifest has no %q entry", umaDatasetKey)
	}

	body, err = get(fmt.Sprintf(umaDatasetURL, umaDatasetKey, hash))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var cards []umaCard
	if err := json.NewDecoder(body).Decode(&cards); err != nil {
		return nil, fmt.Errorf("parse uma %s: %w", umaDatasetKey, err)
	}
	return cards, nil
}

// buildUmaCharacters turns outfit cards into one entry per character
// (char_id), newest first: a lower char_id is an older character. The name is
// the English name (Japanese if there is none), plus the Japanese, Korean and
// Traditional Chinese names.
func buildUmaCharacters(cards []umaCard) []Character {
	type entry struct {
		id               int
		name, jp, ko, tw string
	}
	byID := map[int]*entry{}
	for _, c := range cards {
		id := int(c.CharID)
		if id == 0 {
			continue
		}
		e := byID[id]
		if e == nil {
			e = &entry{id: id}
			byID[id] = e
		}
		// Cards of one character agree; fill any gaps from later cards.
		for _, f := range []struct {
			dst *string
			src *string
		}{{&e.name, c.NameEN}, {&e.jp, c.NameJP}, {&e.ko, c.NameKO}, {&e.tw, c.NameTW}} {
			if *f.dst == "" {
				*f.dst = deref(f.src)
			}
		}
	}

	list := make([]*entry, 0, len(byID))
	for _, e := range byID {
		if e.name == "" {
			e.name = e.jp // no English name yet: fall back to the Japanese one
		}
		if e.name == "" {
			continue
		}
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id > list[j].id }) // newest first

	chars := make([]Character, 0, len(list))
	pos := map[string]int{}
	for _, e := range list {
		c := newNameBuilder().
			Name(LangEN, e.name).
			Name(LangJA, e.jp).
			Name(LangKO, e.ko).
			Name(LangTW, e.tw).
			Character()
		key := strings.ToLower(c.Name[LangEN])
		if p, ok := pos[key]; ok { // same name under another id: fold into the newer one
			chars[p] = builderOf(chars[p]).Merge(c).Character()
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, c)
	}
	return chars
}

// Scrape skips everything when the UmaTools commit hasn't changed since the
// last run (cached in config/commits.json), on the assumption that UmaTools is
// updated whenever new characters are added. Otherwise the whole list is
// rebuilt from GameTora.
func (u UmaMusume) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(umaRepo, umaBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(u.Name(), sha, existing) {
		log.Printf("[Uma Musume] UmaTools unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	cards, err := fetchUmaCards()
	if err != nil {
		return nil, err
	}
	chars := buildUmaCharacters(cards)
	if len(chars) == 0 {
		return nil, fmt.Errorf("uma musume: no characters found")
	}
	if err := SaveCommit(u.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Uma Musume] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
