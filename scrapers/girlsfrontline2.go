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
// has English names; names in other languages are added to data/ by hand, so
// existing entries are never overwritten, only new dolls are merged in. Names
// in other languages can be added to data/ by hand under "name" (ja_jp, ...).
type GirlsFrontline2 struct{}

func (GirlsFrontline2) Name() string { return "Girls Frontline 2" }

// Scrape skips everything when the gfl2-team-builder commit hasn't changed
// since the last run (cached in config/commits.json). Otherwise it fetches the
// doll list (newest first, a higher gunDataId is a later release) and merges
// it into the existing data: dolls already present keep their saved entry
// (name and hand-written aliases) and new dolls are put at the front.
func (g GirlsFrontline2) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(gfl2Repo, gfl2Branch)
	if err != nil {
		return nil, err
	}
	// Not upToDate(): the source has English names only, so there is nothing to
	// rebuild when the saved list is in the old format (it is converted on load).
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

	// Names already saved, including their aliases, so a doll that is known
	// under any of them is not added a second time.
	known := map[string]bool{}
	for _, c := range existing {
		for _, n := range c.Name {
			known[strings.ToLower(n)] = true
		}
		for _, a := range c.Aliases {
			known[strings.ToLower(strings.TrimSpace(a))] = true
		}
	}

	// idx is newest first, so new dolls keep that order at the front.
	added := make([]Character, 0)
	for _, i := range idx {
		name := strings.Join(strings.Fields(raw.Dolls[i].Name), " ")
		k := strings.ToLower(name)
		if known[k] {
			continue
		}
		known[k] = true
		added = append(added, Character{Name: Names{LangEN: name}, Aliases: []string{}})
	}
	if len(existing) == 0 && len(added) == 0 {
		return nil, fmt.Errorf("girls frontline 2: no dolls found")
	}

	chars := make([]Character, 0, len(added)+len(existing))
	chars = append(chars, added...)
	chars = append(chars, existing...)

	if err := SaveCommit(g.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Girls Frontline 2] %d dolls, %d new (commit %s)", len(chars), len(added), sha[:7])
	return chars, nil
}