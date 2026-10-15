package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// atelierURL is the Barrel Wisdom character list of Atelier Resleriana
// (A25) in one language.
const atelierURL = "https://barrelwisdom.com/api/A25/character/%s/"

// atelierLangs maps the language path of the API to its KKAFIO code. "en" is
// the main name; the other lists are matched to it by id.
var atelierLangs = []struct{ path, lang string }{
	{"en", LangEN},
	{"ja", LangJA},
	{"sc", LangZH}, // Simplified Chinese
	{"tc", LangTW}, // Traditional Chinese
}

// Atelier reads the characters of Atelier Resleriana from Barrel Wisdom. The
// game has reached end of service, so the scraper only runs when
// data/Atelier.json does not exist yet; delete the file to scrape again.
type Atelier struct{}

func (Atelier) Name() string { return "Atelier" }

type atelierCharacter struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func fetchAtelier(path string) ([]atelierCharacter, error) {
	body, err := get(fmt.Sprintf(atelierURL, path))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var list []atelierCharacter
	if err := json.NewDecoder(body).Decode(&list); err != nil {
		return nil, fmt.Errorf("parse atelier %s characters: %w", path, err)
	}
	return list, nil
}

// buildAtelierCharacters makes one entry per English name. The English list is
// sorted newest to oldest and gives the order; an entry's other names are
// matched by id. Several versions of one character (same name, different
// title) fold into one entry placed at the position of the oldest version.
func buildAtelierCharacters(lists map[string][]atelierCharacter) []Character {
	byLang := map[string]map[int]string{} // lang -> id -> name
	for lang, list := range lists {
		m := make(map[int]string, len(list))
		for _, c := range list {
			m[c.ID] = c.Name
		}
		byLang[lang] = m
	}

	en := lists[LangEN]
	var oldest []Character
	pos := map[string]int{}             // lowercase en name -> index in oldest
	for i := len(en) - 1; i >= 0; i-- { // oldest first
		c := en[i]
		name := strings.Join(strings.Fields(c.Name), " ")
		if name == "" {
			continue
		}
		b := newNameBuilder().Name(LangEN, name)
		for _, l := range atelierLangs[1:] {
			b.Name(l.lang, byLang[l.lang][c.ID])
		}
		built := b.Character()
		// The same character has the same Japanese name in every version, but
		// the English list falls back to Japanese for the newest characters, so
		// fold by the Japanese name when there is one.
		key := strings.ToLower(name)
		if ja := strings.TrimSpace(byLang[LangJA][c.ID]); ja != "" {
			key = "ja:" + strings.ToLower(ja)
		}
		if p, ok := pos[key]; ok {
			oldest[p] = builderOf(oldest[p]).Merge(built).Character()
			continue
		}
		pos[key] = len(oldest)
		oldest = append(oldest, built)
	}

	chars := make([]Character, 0, len(oldest)) // newest first
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}
	return chars
}

func (a Atelier) Scrape(existing []Character) ([]Character, error) {
	if len(existing) > 0 {
		log.Printf("[Atelier] game has ended and data/Atelier.json exists, skipping")
		return existing, nil
	}
	lists := map[string][]atelierCharacter{}
	for _, l := range atelierLangs {
		list, err := fetchAtelier(l.path)
		if err != nil {
			return nil, err
		}
		lists[l.lang] = list
	}
	chars := buildAtelierCharacters(lists)
	if len(chars) == 0 {
		return nil, fmt.Errorf("atelier: no characters found")
	}
	log.Printf("[Atelier] %d characters", len(chars))
	return chars, nil
}
