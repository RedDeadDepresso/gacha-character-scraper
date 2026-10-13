package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	nteRepo      = "Waifus-Grace/NTE_Assets"
	nteBranch    = "main"
	nteGameURL   = "https://raw.githubusercontent.com/Waifus-Grace/NTE_Assets/refs/heads/main/Localization/%s/game.json"
	ntePlayerURL = "https://raw.githubusercontent.com/Waifus-Grace/NTE_Assets/refs/heads/main/Text/ST_Player.json"
)

// Languages fetched besides "en", in alias order. A language whose game.json
// does not exist (404) is skipped.
var nteLangs = []string{"ja", "ko", "ru", "zh-CN", "zh-Hans", "zh-Hant"}

var nteRoleKey = regexp.MustCompile(`^Role_Name_(\d+)$`)

// NevernessToEverness reads the Role_Name_### entries of the ST_Player string
// table from the NTE_Assets datamine. NPCs in that table are included.
type NevernessToEverness struct{}

func (NevernessToEverness) Name() string { return "Neverness to Everness" }

// Scrape skips everything when the NTE_Assets commit hasn't changed since the
// last run (cached in config/commits.json). Otherwise it rebuilds the list,
// newest-first (a lower Role_Name number is treated as older).
func (n NevernessToEverness) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(nteRepo, nteBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(n.Name()) {
		log.Printf("[Neverness to Everness] NTE_Assets unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	en, err := fetchNTEGame("en")
	if err != nil {
		return nil, err
	}
	if len(en) == 0 {
		return nil, fmt.Errorf("neverness to everness: no Role_Name entries in en game.json")
	}

	names := map[string]map[int]string{} // lang -> role number -> name
	for _, lang := range nteLangs {
		m, err := fetchNTEGame(lang)
		if errors.Is(err, ErrNotFound) {
			log.Printf("[Neverness to Everness] no game.json for %s, skipping", lang)
			continue
		}
		if err != nil {
			return nil, err
		}
		names[lang] = m
	}
	zh, err := fetchNTEChinese()
	if err != nil {
		return nil, err
	}
	names["zh"] = zh
	order := append(append([]string{}, nteLangs...), "zh")

	// Group role numbers that share an English name (e.g. several forms of one character).
	type group struct {
		name string
		ids  []int
	}
	groups := map[string]*group{}
	for id, name := range en {
		k := strings.ToLower(name)
		if g, ok := groups[k]; ok {
			g.ids = append(g.ids, id)
		} else {
			groups[k] = &group{name: name, ids: []int{id}}
		}
	}
	list := make([]*group, 0, len(groups))
	for _, g := range groups {
		sort.Ints(g.ids)
		list = append(list, g)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ids[0] != list[j].ids[0] {
			return list[i].ids[0] > list[j].ids[0] // newest first
		}
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})

	chars := make([]Character, 0, len(list))
	for _, g := range list {
		c := Character{Name: g.name, Aliases: []string{}}
		seen := map[string]bool{strings.ToLower(g.name): true}
		for _, lang := range order {
			for _, id := range g.ids {
				a := names[lang][id]
				if a != "" && !seen[strings.ToLower(a)] {
					seen[strings.ToLower(a)] = true
					c.Aliases = append(c.Aliases, a)
				}
			}
		}
		chars = append(chars, c)
	}

	if err := SaveCommit(n.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Neverness to Everness] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

// fetchNTEGame reads the ST_Player table from Localization/<lang>/game.json.
func fetchNTEGame(lang string) (map[int]string, error) {
	body, err := get(fmt.Sprintf(nteGameURL, lang))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", lang, err)
	}
	defer body.Close()
	// Only decode the ST_Player table; the file has many large tables.
	var tables map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&tables); err != nil {
		return nil, fmt.Errorf("parse %s game.json: %w", lang, err)
	}
	raw, ok := tables["ST_Player"]
	if !ok {
		return nil, fmt.Errorf("%s game.json has no ST_Player table", lang)
	}
	var entries map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse %s ST_Player: %w", lang, err)
	}
	return roleNames(entries), nil
}

// fetchNTEChinese reads Text/ST_Player.json (an Unreal string-table export).
func fetchNTEChinese() (map[int]string, error) {
	body, err := get(ntePlayerURL)
	if err != nil {
		return nil, fmt.Errorf("fetch ST_Player.json: %w", err)
	}
	defer body.Close()
	var doc []struct {
		StringTable struct {
			KeysToEntries map[string]string `json:"KeysToEntries"`
		} `json:"StringTable"`
	}
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse ST_Player.json: %w", err)
	}
	out := map[int]string{}
	for _, d := range doc {
		for id, name := range roleNames(d.StringTable.KeysToEntries) {
			out[id] = name
		}
	}
	return out, nil
}

// roleNames keeps the Role_Name_### entries (not the *_des ones) with a
// non-blank name, keyed by number.
func roleNames(entries map[string]string) map[int]string {
	out := map[int]string{}
	for k, v := range entries {
		m := nteRoleKey.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		v = strings.NewReplacer("「", "", "」", "", "『", "", "』", "").Replace(v)
		if v = strings.Join(strings.Fields(v), " "); v != "" {
			out[id] = v
		}
	}
	return out
}