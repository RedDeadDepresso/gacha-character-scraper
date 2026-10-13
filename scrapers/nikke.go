package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const (
	nikkeRepo   = "williamchong/nikke-arena-planner"
	nikkeBranch = "master"
	nikkeChars  = "https://raw.githubusercontent.com/williamchong/nikke-arena-planner/refs/heads/master/app/data/characters.json"
)

// Languages used for aliases, in alias order.
var nikkeLangs = []string{"zh-TW", "zh-CN", "ja"}

// Nikke reads characters from the nikke-arena-planner repo.
type Nikke struct{}

func (Nikke) Name() string { return "Nikke" }

// Scrape skips everything when the nikke-arena-planner commit hasn't changed
// since the last run (cached in config/commits.json). Otherwise it rebuilds the
// list newest-first, ordered by releaseOrder (higher = newer).
func (n Nikke) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(nikkeRepo, nikkeBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(n.Name()) {
		log.Printf("[Nikke] nikke-arena-planner unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(nikkeChars)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		ID           string            `json:"id"`
		Name         map[string]string `json:"name"`
		ReleaseOrder int               `json:"releaseOrder"`
	}
	err = json.NewDecoder(body).Decode(&entries)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse characters.json: %w", err)
	}

	// releaseOrder is not unique and the file is not sorted, so use the file
	// position as a tie-breaker (later in the file = newer).
	idx := make([]int, 0, len(entries))
	for i, e := range entries {
		if strings.TrimSpace(e.Name["en"]) != "" {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ea, eb := entries[idx[a]], entries[idx[b]]
		if ea.ReleaseOrder != eb.ReleaseOrder {
			return ea.ReleaseOrder > eb.ReleaseOrder
		}
		return idx[a] > idx[b]
	})

	chars := make([]Character, 0, len(idx))
	pos := map[string]int{}
	for _, i := range idx {
		e := entries[i]
		name := strings.Join(strings.Fields(e.Name["en"]), " ")
		var aliases []string
		for _, lang := range nikkeLangs {
			aliases = append(aliases, e.Name[lang])
		}
		key := strings.ToLower(name)
		if p, ok := pos[key]; ok { // same name: fold into the newer entry
			chars[p].Aliases = cleanAliases(chars[p].Name, append(chars[p].Aliases, aliases...))
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("nikke: no characters found")
	}
	if err := SaveCommit(n.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Nikke] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
