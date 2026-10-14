package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

const (
	genshinRepo   = "frzyc/genshin-optimizer"
	genshinBranch = "master"
)

const genshinURL = "https://raw.githubusercontent.com/frzyc/genshin-optimizer/refs/heads/master/libs/gi/dm-localization/assets/locales/%s/charNames_gen.json"

// Languages fetched besides "en", in alias order. All share the same keys. A
// language whose file does not exist (404) is skipped.
var genshinLangs = []string{"ja", "ko", "ru", "chs", "cht"}

// GenshinImpact reads character names from the genshin-optimizer localization
// data (key -> name, one file per language).
type GenshinImpact struct{}

func (GenshinImpact) Name() string { return "Genshin Impact" }

// Scrape skips everything when the genshin-optimizer commit hasn't changed
// since the last run (cached in config/commits.json). Otherwise it rebuilds the
// list. The file lists characters roughly in release order, so the result is
// reversed to newest-first.
func (g GenshinImpact) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(genshinRepo, genshinBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(g.Name()) {
		log.Printf("[Genshin Impact] genshin-optimizer unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	en, order, err := fetchGenshinNames("en")
	if err != nil {
		return nil, err
	}

	names := map[string]map[string]string{} // lang -> key -> name
	for _, lang := range genshinLangs {
		m, _, err := fetchGenshinNames(lang)
		if errors.Is(err, ErrNotFound) {
			log.Printf("[Genshin Impact] no charNames_gen.json for %s, skipping", lang)
			continue
		}
		if err != nil {
			return nil, err
		}
		names[lang] = m
	}

	var oldest []Character
	pos := map[string]int{} // lowercase en name -> index in oldest
	for _, key := range order {
		// TravelerAnemoM, TravelerGeoF, ... are element forms of Aether and
		// Lumine (TravelerM / TravelerF), not separate characters.
		if strings.HasPrefix(key, "Traveler") && key != "TravelerM" && key != "TravelerF" {
			continue
		}
		name := en[key]
		var aliases []string
		for _, lang := range genshinLangs {
			aliases = append(aliases, names[lang][key])
		}
		k := strings.ToLower(name)
		if p, ok := pos[k]; ok {
			oldest[p].Aliases = cleanAliases(oldest[p].Name, append(oldest[p].Aliases, aliases...))
			continue
		}
		pos[k] = len(oldest)
		oldest = append(oldest, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	if len(oldest) == 0 {
		return nil, fmt.Errorf("genshin impact: no characters found")
	}

	chars := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}
	if err := SaveCommit(g.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Genshin Impact] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

// fetchGenshinNames reads one language file, returning key -> name and the keys
// in file order (a Go map would lose it). Entries with an empty name are dropped.
func fetchGenshinNames(lang string) (map[string]string, []string, error) {
	body, err := get(fmt.Sprintf(genshinURL, lang))
	if err != nil {
		return nil, nil, err
	}
	defer body.Close()

	dec := json.NewDecoder(body)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, fmt.Errorf("genshin %s: expected a JSON object", lang)
	}
	m := map[string]string{}
	var order []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("parse genshin %s: %w", lang, err)
		}
		key, _ := t.(string)
		var name string
		if err := dec.Decode(&name); err != nil {
			return nil, nil, fmt.Errorf("parse genshin %s: %w", lang, err)
		}
		name = strings.Join(strings.Fields(name), " ")
		if key != "" && name != "" {
			m[key] = name
			order = append(order, key)
		}
	}
	return m, order, nil
}
