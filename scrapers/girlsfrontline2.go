package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const (
	gfl2Repo   = "Infernal-Crack-LED/gfl2-team-builder"
	gfl2Branch = "main"
	gfl2URL    = "https://raw.githubusercontent.com/Infernal-Crack-LED/gfl2-team-builder/refs/heads/main/data/dolls.json"
)

// GirlsFrontline2 reads dolls from the gfl2-team-builder open game data. The
// name has no apostrophe or colon so it is safe as a file name. The source only
// has English names, so there are no aliases.
type GirlsFrontline2 struct{}

func (GirlsFrontline2) Name() string { return "Girls Frontline 2" }

// Scrape skips everything when the gfl2-team-builder commit hasn't changed
// since the last run (cached in config/commits.json). Otherwise it rebuilds the
// list newest-first (a higher gunDataId is a later release).
func (g GirlsFrontline2) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(gfl2Repo, gfl2Branch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(g.Name()) {
		log.Printf("[Girls Frontline 2] gfl2-team-builder unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(gfl2URL)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Dolls []struct {
			Name      string `json:"name"`
			GunDataID int    `json:"gunDataId"`
		} `json:"dolls"`
	}
	err = json.NewDecoder(body).Decode(&raw)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse gfl2 dolls.json: %w", err)
	}

	// Newest first; the file position breaks ties (later = newer).
	idx := make([]int, 0, len(raw.Dolls))
	for i, d := range raw.Dolls {
		if strings.TrimSpace(d.Name) != "" {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		da, db := raw.Dolls[idx[a]], raw.Dolls[idx[b]]
		if da.GunDataID != db.GunDataID {
			return da.GunDataID > db.GunDataID
		}
		return idx[a] > idx[b]
	})

	chars := make([]Character, 0, len(idx))
	seen := map[string]bool{}
	for _, i := range idx {
		name := strings.Join(strings.Fields(raw.Dolls[i].Name), " ")
		if k := strings.ToLower(name); !seen[k] {
			seen[k] = true
			chars = append(chars, Character{Name: name, Aliases: []string{}})
		}
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("girls frontline 2: no dolls found")
	}

	if err := SaveCommit(g.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Girls Frontline 2] %d dolls (commit %s)", len(chars), sha[:7])
	return chars, nil
}
