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
	zzzRepo   = "a285292107s/zzz-wiki"
	zzzBranch = "master"
	zzzURL    = "https://raw.githubusercontent.com/a285292107s/zzz-wiki/refs/heads/master/public/data/live/character.json"
)

// ZenlessZoneZero reads character names from the zzz-wiki repo.
type ZenlessZoneZero struct{}

func (ZenlessZoneZero) Name() string { return "Zenless Zone Zero" }

type zzzNames struct {
	EN string `json:"en"`
	KO string `json:"ko"`
	ZH string `json:"zh"`
	JA string `json:"ja"`
}

// Scrape skips everything when the zzz-wiki commit hasn't changed since the
// last run (cached in config/commits.json). Otherwise it rebuilds the list,
// newest-first (lower id = older).
func (z ZenlessZoneZero) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(zzzRepo, zzzBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(z.Name(), sha, existing) {
		log.Printf("[Zenless Zone Zero] zzz-wiki unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(zzzURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw := map[string]zzzNames{}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse zzz character.json: %w", err)
	}

	type entry struct {
		id int
		zzzNames
	}
	best := map[string]entry{} // lowercase en name -> lowest id
	for key, n := range raw {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		n.EN = strings.TrimSpace(n.EN)
		if n.EN == "" {
			continue
		}
		k := strings.ToLower(n.EN)
		if cur, ok := best[k]; !ok || id < cur.id {
			best[k] = entry{id: id, zzzNames: n}
		}
	}

	list := make([]entry, 0, len(best))
	for _, e := range best {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id > list[j].id }) // newest first

	chars := make([]Character, 0, len(list))
	for _, e := range list {
		chars = append(chars, newNameBuilder().
			Name(LangEN, e.EN).
			Name(LangZH, e.ZH).
			Name(LangJA, e.JA).
			Name(LangKO, e.KO).
			Character())
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("zenless zone zero: no characters found")
	}

	if err := SaveCommit(z.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Zenless Zone Zero] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
