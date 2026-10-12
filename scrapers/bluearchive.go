package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const (
	schaleRepo    = "nico0748/SchaleDB"
	schaleBranch  = "main"
	schaleDataURL = "https://raw.githubusercontent.com/nico0748/SchaleDB/refs/heads/main/data/%s/students.min.json"
)

// Languages in SchaleDB's data folder. "en" is the primary one.
var schaleLangs = []string{"en", "jp", "kr", "tw", "zh"}

// BlueArchive builds its list from SchaleDB's student data.
type BlueArchive struct{}

func (BlueArchive) Name() string { return "Blue Archive" }

type schaleStudent struct {
	Id           int    `json:"Id"`
	IsReleased   []bool `json:"IsReleased"`
	Name         string `json:"Name"`
	FamilyName   string `json:"FamilyName"`
	PersonalName string `json:"PersonalName"`
}

func (s schaleStudent) released() bool {
	for _, r := range s.IsReleased {
		if r {
			return true
		}
	}
	return false
}

// Scrape skips everything when the SchaleDB commit hasn't changed since the
// last run (cached in config/commits.json). Otherwise it rebuilds the list.
func (b BlueArchive) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(schaleRepo, schaleBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(b.Name()) {
		log.Printf("[Blue Archive] SchaleDB unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	byLang := map[string]map[string]schaleStudent{}
	for _, lang := range schaleLangs {
		m, err := fetchSchale(lang)
		if err != nil {
			return nil, err
		}
		byLang[lang] = m
	}

	// Group variants (swimsuit, new year, ...) by their English family + personal
	// name and keep the lowest Id (the original release) as the base.
	type group struct{ base schaleStudent }
	groups := map[string]*group{}
	for _, s := range byLang["en"] {
		if !s.released() {
			continue
		}
		key := strings.ToLower(s.FamilyName + "|" + s.PersonalName)
		if s.FamilyName == "" && s.PersonalName == "" {
			key = strings.ToLower(s.Name)
		}
		if g, ok := groups[key]; !ok || s.Id < g.base.Id {
			groups[key] = &group{base: s}
		}
	}

	bases := make([]schaleStudent, 0, len(groups))
	for _, g := range groups {
		bases = append(bases, g.base)
	}
	sort.Slice(bases, func(i, j int) bool { return bases[i].Id > bases[j].Id }) // newest first

	chars := make([]Character, 0, len(bases))
	for _, base := range bases {
		c := Character{Name: displayName(base), Aliases: []string{}}
		seen := map[string]bool{strings.ToLower(c.Name): true}
		add := func(a string) {
			a = strings.TrimSpace(a)
			if a == "" || seen[strings.ToLower(a)] {
				return
			}
			seen[strings.ToLower(a)] = true
			c.Aliases = append(c.Aliases, a)
		}
		for _, lang := range schaleLangs {
			s, ok := byLang[lang][fmt.Sprint(base.Id)]
			if !ok {
				continue
			}
			for _, a := range langAliases(lang, s) {
				add(a)
			}
		}
		chars = append(chars, c)
	}

	if err := SaveCommit(b.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Blue Archive] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

func fetchSchale(lang string) (map[string]schaleStudent, error) {
	body, err := get(fmt.Sprintf(schaleDataURL, lang))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", lang, err)
	}
	defer body.Close()
	m := map[string]schaleStudent{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", lang, err)
	}
	return m, nil
}

// displayName returns "Family, Personal" (KKAFIO order), or a single word.
func displayName(s schaleStudent) string {
	switch {
	case s.FamilyName != "" && s.PersonalName != "":
		return s.FamilyName + ", " + s.PersonalName
	case s.PersonalName != "":
		return s.PersonalName
	case s.FamilyName != "":
		return s.FamilyName
	}
	return s.Name
}

// langAliases returns the name forms of a student in one language.
func langAliases(lang string, s schaleStudent) []string {
	var out []string
	if s.FamilyName != "" && s.PersonalName != "" {
		if lang == "en" {
			out = append(out, s.PersonalName+" "+s.FamilyName) // Western order
		} else {
			out = append(out, s.FamilyName+s.PersonalName, s.FamilyName+" "+s.PersonalName)
		}
	}
	// The in-game short name, unless it is a variant such as "Hina (Swimsuit)".
	if !strings.ContainsAny(s.Name, "(（") {
		out = append(out, s.Name)
	}
	return out
}
