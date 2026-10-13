package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	hsrRepo   = "fribbels/hsr-optimizer"
	hsrBranch = "main"

	hsrDataURL   = "https://raw.githubusercontent.com/fribbels/hsr-optimizer/refs/heads/main/src/data/game_data.json"
	hsrLocaleURL = "https://raw.githubusercontent.com/fribbels/hsr-optimizer/refs/heads/main/public/locales/%s/gameData.yaml"
)

// Locale folders used for aliases, in alias order.
var hsrLangs = []string{"ja_JP", "ko_KR", "ru_RU", "zh_CN", "zh_TW"}

// HonkaiStarRail reads characters from the hsr-optimizer repo.
type HonkaiStarRail struct{}

func (HonkaiStarRail) Name() string { return "Honkai Star Rail" }

type hsrNames struct {
	Name     string `yaml:"Name"`
	LongName string `yaml:"LongName"`
}

// Scrape skips everything when the hsr-optimizer commit hasn't changed since the
// last run (cached in config/commits.json). Otherwise it rebuilds the list,
// newest-first (lower id = older).
func (h HonkaiStarRail) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(hsrRepo, hsrBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(h.Name()) {
		log.Printf("[Honkai Star Rail] hsr-optimizer unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(hsrDataURL)
	if err != nil {
		return nil, err
	}
	var data struct {
		Characters map[string]struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"characters"`
	}
	err = json.NewDecoder(body).Decode(&data)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse game_data.json: %w", err)
	}

	locales := map[string]map[string]hsrNames{} // lang -> id -> names
	for _, lang := range hsrLangs {
		m, err := fetchHSRLocale(lang)
		if err != nil {
			return nil, err
		}
		locales[lang] = m
	}

	type entry struct {
		order int
		id    string
		name  string
	}
	best := map[string]entry{} // lowercase name -> lowest id
	for key, c := range data.Characters {
		id := c.ID
		if id == "" {
			id = key
		}
		name := cleanHSR(c.Name)
		if name == "" {
			continue
		}
		order := 1 << 30 // non-numeric id: treat as newest
		if n, err := strconv.Atoi(id); err == nil {
			order = n
		}
		k := strings.ToLower(name)
		if cur, ok := best[k]; !ok || order < cur.order {
			best[k] = entry{order: order, id: id, name: name}
		}
	}

	list := make([]entry, 0, len(best))
	for _, e := range best {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].order != list[j].order {
			return list[i].order > list[j].order // newest first
		}
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})

	chars := make([]Character, 0, len(list))
	for _, e := range list {
		c := Character{Name: e.name, Aliases: []string{}}
		seen := map[string]bool{strings.ToLower(e.name): true}
		add := func(a string) {
			a = cleanHSR(a)
			if a != "" && !seen[strings.ToLower(a)] {
				seen[strings.ToLower(a)] = true
				c.Aliases = append(c.Aliases, a)
			}
		}
		for _, lang := range hsrLangs {
			n := locales[lang][e.id]
			add(n.Name)
			add(n.LongName) // skipped automatically when identical to Name
		}
		chars = append(chars, c)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("honkai star rail: no characters found")
	}
	if err := SaveCommit(h.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Honkai Star Rail] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

func fetchHSRLocale(lang string) (map[string]hsrNames, error) {
	body, err := get(fmt.Sprintf(hsrLocaleURL, lang))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", lang, err)
	}
	defer body.Close()
	var doc struct {
		Characters map[string]hsrNames `yaml:"Characters"`
	}
	if err := yaml.NewDecoder(body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", lang, err)
	}
	return doc.Characters, nil
}

// cleanHSR removes the ⚰️ emoji (with or without its variation selector) and
// surrounding whitespace.
func cleanHSR(s string) string {
	s = strings.ReplaceAll(s, "\u26b0\ufe0f", "")
	s = strings.ReplaceAll(s, "\u26b0", "")
	return strings.Join(strings.Fields(s), " ")
}