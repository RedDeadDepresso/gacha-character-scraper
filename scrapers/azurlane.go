package scrapers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	azurRepo   = "xyz-alpha/AzurLaneFleet"
	azurBranch = "master"
	azurURL    = "https://raw.githubusercontent.com/xyz-alpha/AzurLaneFleet/refs/heads/master/js/ship_data.min.js"
)

// AzurLane reads ship_data.min.js from AzurLaneFleet, a JavaScript file that
// is just `const ship_data={...};`.
type AzurLane struct{}

func (AzurLane) Name() string { return "Azur Lane" }

// looseString reads a JSON string or number as a string.
type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	var str string
	if json.Unmarshal(b, &str) == nil {
		*s = looseString(str)
		return nil
	}
	*s = looseString(strings.Trim(string(b), `"`))
	return nil
}

// azurShip holds the fields of one ship_data entry that we use.
type azurShip struct {
	EN   looseString `json:"en_name"`
	EN2  looseString `json:"english_name"` // used when en_name is empty
	CN   looseString `json:"cn_name"`
	TW   looseString `json:"tw_name"`
	JP   looseString `json:"jp_name"`
	Date looseString `json:"date"` // release date, e.g. 2026年07月16日
	KR   looseString `json:"kr_name"`
}

// parseAzurShips converts the contents of ship_data.min.js to a map keyed by
// the ship's id as written in the file (e.g. "100000", or "1e5" for 100000).
func parseAzurShips(src string) (map[string]azurShip, error) {
	b, err := jsObjectToJSON(src, "ship_data")
	if err != nil {
		return nil, fmt.Errorf("azur lane ship_data: %w", err)
	}
	ships := map[string]azurShip{}
	if err := json.Unmarshal(b, &ships); err != nil {
		return nil, fmt.Errorf("parse azur lane ship_data: %w", err)
	}
	return ships, nil
}

// azurName trims a name and the trailing "*" that marks some collab ships in
// the source.
func azurName(s looseString) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(string(s)), "*"))
}

var azurDateRe = regexp.MustCompile(`(\d{4})\D+(\d{1,2})(?:\D+(\d{1,2}))?`)

// azurDate converts a release date such as "2026年07月16日" or "2017年06月" to
// a sortable number (YYYYMMDD); 0 when there is no date.
func azurDate(s string) int {
	m := azurDateRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	return y*10000 + mo*100 + d
}

// mergeAzur folds b into a: every name of b becomes an alias of a, even in a
// language where a has no name (that name would belong to a different
// character). Languages are visited in a fixed order (unlike
// nameBuilder.Merge, which ranges over a map) so that the aliases, and the
// saved file, come out the same on every run.
func mergeAzur(a, b Character) Character {
	nb := builderOf(a)
	for _, l := range []string{LangEN, LangZH, LangTW, LangJA, LangKO, LangRU} {
		nb.Alias(b.Name[l])
	}
	return nb.Alias(b.Aliases...).Character()
}

// buildAzurCharacters makes the characters, newest first by release date
// (then by id). Retrofit versions ("X (Retrofit)") are separate entries in the
// source and are skipped. Entries that share an English name (skins, or
// unrelated collab ships such as the two Fubukis) become one character placed
// at the oldest of them; the names of the others become aliases.
func buildAzurCharacters(ships map[string]azurShip) []Character {
	type entry struct {
		date int
		id   float64
		char Character
	}
	var list []entry
	for key, s := range ships {
		name := azurName(s.EN)
		if name == "" {
			name = azurName(s.EN2)
		}
		if name == "" || strings.HasSuffix(name, "(Retrofit)") {
			continue
		}
		id, err := strconv.ParseFloat(strings.TrimSpace(key), 64)
		if err != nil {
			id = math.Inf(1) // non-numeric ids count as newest
		}
		cn := azurName(s.CN)
		b := newNameBuilder().
			Name(LangEN, name).
			Name(LangZH, cn).
			Name(LangJA, azurName(s.JP)).
			Name(LangKO, azurName(s.KR))
		// Traditional Chinese is only kept when it differs from Simplified.
		if tw := azurName(s.TW); tw != cn {
			b.Name(LangTW, tw)
		}
		list = append(list, entry{azurDate(string(s.Date)), id, b.Character()})
	}
	sort.Slice(list, func(i, j int) bool { // oldest first
		a, b := list[i], list[j]
		if a.date != b.date {
			return a.date < b.date
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.char.Name.EN() < b.char.Name.EN()
	})

	var oldest []Character
	pos := map[string]int{} // lowercase en name -> index in oldest
	for _, e := range list {
		key := strings.ToLower(e.char.Name.EN())
		if p, ok := pos[key]; ok {
			oldest[p] = mergeAzur(oldest[p], e.char)
			continue
		}
		pos[key] = len(oldest)
		oldest = append(oldest, e.char)
	}
	chars := make([]Character, 0, len(oldest)) // newest first
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}
	return chars
}

// Scrape skips everything when the AzurLaneFleet commit hasn't changed since
// the last run (cached in config/commits.json). Otherwise it downloads
// ship_data.min.js and rebuilds the list, newest first.
func (a AzurLane) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(azurRepo, azurBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(a.Name(), sha, existing) {
		log.Printf("[Azur Lane] AzurLaneFleet unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(azurURL)
	if err != nil {
		return nil, err
	}
	src, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	ships, err := parseAzurShips(string(src))
	if err != nil {
		return nil, err
	}
	chars := buildAzurCharacters(ships)
	if len(chars) == 0 {
		return nil, fmt.Errorf("azur lane: no ships found")
	}
	if err := SaveCommit(a.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Azur Lane] %d ships (commit %s)", len(chars), sha[:7])
	return chars, nil
}
