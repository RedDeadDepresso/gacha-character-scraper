package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const (
	wuwaRepo = "trungtq2311/wuwa-companion"
	// "HEAD" resolves to the repo's default branch in the GitHub commits API.
	wuwaRef = "HEAD"
	wuwaURL = "https://api-v2.encore.moe/api/%s/character"
)

// Languages for the encore.moe API; "en" is the primary one, the rest become aliases.
var wuwaLangs = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"}

// WutheringWaves reads the character list from the encore.moe API.
type WutheringWaves struct{}

func (WutheringWaves) Name() string { return "Wuthering Waves" }

type wuwaResponse struct {
	RoleList []struct {
		ID   int    `json:"Id"`
		Name string `json:"Name"`
	} `json:"roleList"`
}

// Scrape skips everything when the wuwa-companion commit hasn't changed since
// the last run (cached in config/commits.json). Otherwise it rebuilds the list,
// newest-first (lower Id = older).
func (w WutheringWaves) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(wuwaRepo, wuwaRef)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(w.Name()) {
		log.Printf("[Wuthering Waves] wuwa-companion unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	names := map[string]map[int]string{} // lang -> id -> name
	for _, lang := range wuwaLangs {
		m, err := fetchWuwa(lang)
		if err != nil {
			return nil, err
		}
		names[lang] = m
	}

	type entry struct {
		id   int
		name string
	}
	best := map[string]entry{} // lowercase en name -> lowest id
	for id, name := range names["en"] {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		k := strings.ToLower(name)
		if cur, ok := best[k]; !ok || id < cur.id {
			best[k] = entry{id: id, name: name}
		}
	}
	list := make([]entry, 0, len(best))
	for _, e := range best {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id > list[j].id }) // newest first

	chars := make([]Character, 0, len(list))
	for _, e := range list {
		c := Character{Name: e.name, Aliases: []string{}}
		seen := map[string]bool{strings.ToLower(e.name): true}
		for _, lang := range wuwaLangs[1:] {
			a := strings.TrimSpace(names[lang][e.id])
			if a != "" && !seen[strings.ToLower(a)] {
				seen[strings.ToLower(a)] = true
				c.Aliases = append(c.Aliases, a)
			}
		}
		chars = append(chars, c)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("wuthering waves: no characters found")
	}

	if err := SaveCommit(w.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Wuthering Waves] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

func fetchWuwa(lang string) (map[int]string, error) {
	body, err := get(fmt.Sprintf(wuwaURL, lang))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", lang, err)
	}
	defer body.Close()
	var resp wuwaResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("parse %s: %w", lang, err)
	}
	m := make(map[int]string, len(resp.RoleList))
	for _, r := range resp.RoleList {
		m[r.ID] = r.Name
	}
	return m, nil
}
