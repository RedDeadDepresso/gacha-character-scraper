package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const (
	holoRepo     = "HolodoriDB/holodori-db-eng-diff"
	holoBranch   = "main"
	holoCharsURL = "https://raw.githubusercontent.com/HolodoriDB/holodori-db-eng-diff/refs/heads/main/Character.json"
	holoLangURL  = "https://raw.githubusercontent.com/HolodoriDB/holodori-db-%s-diff/refs/heads/main/LangCharacter_%s.json"
	holoNamePfx  = "la-name-"
	holoShortPfx = "la-short_name-"
)

// Languages used for aliases, in alias order. The file name uses the
// capitalised language (LangCharacter_Jpn.json).
var holoLangs = []string{"jpn", "kor", "chs", "cht"}

// Hololive reads characters from the Holodori database. The game is saved as
// "Hololive" (data/Hololive.json).
type Hololive struct{}

func (Hololive) Name() string { return "Hololive" }

// holoLangTexts downloads LangCharacter_<Lang>.json and returns the full names
// and short names by character id (the part after "la-name-" / "la-short_name-").
func holoLangTexts(lang string) (names, shorts map[string]string, err error) {
	capLang := strings.ToUpper(lang[:1]) + lang[1:]
	body, err := get(fmt.Sprintf(holoLangURL, lang, capLang))
	if err != nil {
		return nil, nil, err
	}
	defer body.Close()
	var msgs []struct {
		ID   string `json:"id"`
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&msgs); err != nil {
		return nil, nil, fmt.Errorf("parse LangCharacter_%s.json: %w", capLang, err)
	}
	names, shorts = map[string]string{}, map[string]string{}
	for _, m := range msgs {
		text := strings.TrimSpace(m.Data.Text)
		if text == "" {
			continue
		}
		switch {
		case strings.HasPrefix(m.ID, holoNamePfx):
			names[strings.TrimPrefix(m.ID, holoNamePfx)] = text
		case strings.HasPrefix(m.ID, holoShortPfx):
			shorts[strings.TrimPrefix(m.ID, holoShortPfx)] = text
		}
	}
	return names, shorts, nil
}

// Scrape skips everything when the holodori-db-eng-diff commit hasn't changed
// since the last run (cached in config/commits.json). Otherwise it rebuilds the
// list newest-first, ordered by debut date (the "order" field breaks ties, a
// higher value = later). Aliases are the full names and then the short names
// in Japanese, Korean, Simplified and Traditional Chinese, plus the English
// short name.
func (h Hololive) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(holoRepo, holoBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(h.Name()) {
		log.Printf("[Hololive] holodori-db-eng-diff unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(holoCharsURL)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		ID   string `json:"id"`
		Data struct {
			NameEng      string `json:"nameEng"`
			ShortNameEng string `json:"shortNameEng"`
			DebutYear    int    `json:"debutYear"`
			DebutMonth   int    `json:"debutMonth"`
			DebutDay     int    `json:"debutDay"`
			Order        int    `json:"order"`
		} `json:"data"`
	}
	err = json.NewDecoder(body).Decode(&entries)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse Character.json: %w", err)
	}

	// Per language: id -> full name / short name.
	fullNames := make([]map[string]string, len(holoLangs))
	shortNames := make([]map[string]string, len(holoLangs))
	for i, lang := range holoLangs {
		if fullNames[i], shortNames[i], err = holoLangTexts(lang); err != nil {
			return nil, err
		}
	}

	date := func(i int) int {
		d := entries[i].Data
		return d.DebutYear*10000 + d.DebutMonth*100 + d.DebutDay
	}
	idx := make([]int, 0, len(entries))
	for i, e := range entries {
		if strings.TrimSpace(e.Data.NameEng) != "" {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		i, j := idx[a], idx[b]
		if date(i) != date(j) {
			return date(i) > date(j)
		}
		if entries[i].Data.Order != entries[j].Data.Order {
			return entries[i].Data.Order > entries[j].Data.Order
		}
		return i > j
	})

	chars := make([]Character, 0, len(idx))
	pos := map[string]int{}
	for _, i := range idx {
		e := entries[i]
		name := strings.Join(strings.Fields(e.Data.NameEng), " ")
		var aliases []string
		for l := range holoLangs {
			aliases = append(aliases, fullNames[l][e.ID])
		}
		aliases = append(aliases, e.Data.ShortNameEng)
		for l := range holoLangs {
			aliases = append(aliases, shortNames[l][e.ID])
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
		return nil, fmt.Errorf("hololive: no characters found")
	}
	if err := SaveCommit(h.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Hololive] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
