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
	pokemonRepo   = "abcboy101/masters-text"
	pokemonBranch = "main"
	pokemonURL    = "https://raw.githubusercontent.com/abcboy101/masters-text/refs/heads/main/Messages/trainer_name_%s.json"
)

// Languages used for aliases, in alias order. English is the main name.
var pokemonLangs = []string{"ja", "ko", "zh-TW"}

// PokemonMasters reads trainer names from the masters-text repo. The game is
// saved as "Pokemon" (data/Pokemon.json).
type PokemonMasters struct{}

func (PokemonMasters) Name() string { return "Pokemon" }

// fetchPokemonNames downloads trainer_name_<lang>.json (id -> name).
func fetchPokemonNames(lang string) (map[string]string, error) {
	body, err := get(fmt.Sprintf(pokemonURL, lang))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	m := map[string]string{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		return nil, fmt.Errorf("parse trainer_name_%s.json: %w", lang, err)
	}
	return m, nil
}

// Scrape skips everything when the masters-text commit hasn't changed since
// the last run (cached in config/commits.json). Otherwise it rebuilds the list
// newest-first: ids look like "ch0123" and a lower number is an older
// character. Entries with an empty English name are skipped. When the same
// English name appears under several ids (e.g. "Red"), the entries are folded
// into one at the position of the oldest id.
func (p PokemonMasters) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(pokemonRepo, pokemonBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(p.Name()) {
		log.Printf("[Pokemon] masters-text unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	en, err := fetchPokemonNames("en")
	if err != nil {
		return nil, err
	}
	others := map[string]map[string]string{}
	for _, lang := range pokemonLangs {
		if others[lang], err = fetchPokemonNames(lang); err != nil {
			return nil, err
		}
	}

	type entry struct {
		key string
		num int
	}
	ids := make([]entry, 0, len(en))
	for k, name := range en {
		if strings.TrimSpace(name) == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(k, "ch"))
		if err != nil {
			continue // not a "chNNNN" id
		}
		ids = append(ids, entry{k, n})
	}
	// Oldest first, so the first entry seen for a name is the oldest one.
	sort.Slice(ids, func(i, j int) bool { return ids[i].num < ids[j].num })

	var chars []Character // oldest first for now
	pos := map[string]int{}
	for _, e := range ids {
		name := strings.Join(strings.Fields(en[e.key]), " ")
		var aliases []string
		for _, lang := range pokemonLangs {
			aliases = append(aliases, others[lang][e.key])
		}
		key := strings.ToLower(name)
		if i, ok := pos[key]; ok {
			chars[i].Aliases = cleanAliases(chars[i].Name, append(chars[i].Aliases, aliases...))
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("pokemon: no trainers found")
	}
	// Newest first.
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}

	if err := SaveCommit(p.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Pokemon] %d trainers (commit %s)", len(chars), sha[:7])
	return chars, nil
}
