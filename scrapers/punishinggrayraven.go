package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

const (
	pgrRepo   = "alt3ri/PGR_Data"
	pgrBranch = "master"
	pgrURL    = "https://raw.githubusercontent.com/alt3ri/PGR_Data/refs/heads/master/%s/bytes/share/character/Character.json"
)

// PunishingGrayRaven reads Constructs from alt3ri/PGR_Data.
type PunishingGrayRaven struct{}

func (PunishingGrayRaven) Name() string { return "Punishing Gray Raven" }

type pgrCharacter struct {
	ID      int    `json:"Id"`
	Name    string `json:"Name"`
	LogName string `json:"LogName"`
	EnName  string `json:"EnName"`
}

// fetchPGR downloads <lang>/.../Character.json, keeping the file order.
func fetchPGR(lang string) ([]pgrCharacter, error) {
	body, err := get(fmt.Sprintf(pgrURL, lang))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var list []pgrCharacter
	if err := json.NewDecoder(body).Decode(&list); err != nil {
		return nil, fmt.Errorf("parse %s Character.json: %w", lang, err)
	}
	return list, nil
}

// Scrape skips everything when the PGR_Data commit hasn't changed since the
// last run (cached in config/commits.json). Every Id is one entry (a construct
// or outfit), named after its English LogName (e.g. "Lee: Palefire"). Aliases
// are the English Name and EnName, then the Chinese Name, LogName and EnName of
// the same Id. TradeName is not used.
//
// Ids are not a reliable release order (e.g. Lucia: Inverse Crown is 1021007
// but is one of the newest), while the file itself is in release order, so the
// list is the file order reversed, newest first.
func (p PunishingGrayRaven) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(pgrRepo, pgrBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(p.Name()) {
		log.Printf("[Punishing Gray Raven] PGR_Data unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	en, err := fetchPGR("EN")
	if err != nil {
		return nil, err
	}
	zhList, err := fetchPGR("ZH")
	if err != nil {
		return nil, err
	}
	zh := make(map[int]pgrCharacter, len(zhList))
	for _, c := range zhList {
		zh[c.ID] = c
	}

	chars := make([]Character, 0, len(en))
	pos := map[string]int{}
	for i := len(en) - 1; i >= 0; i-- { // newest first
		c := en[i]
		name := strings.Join(strings.Fields(c.LogName), " ")
		if name == "" {
			name = strings.Join(strings.Fields(c.Name), " ")
		}
		if name == "" {
			continue
		}
		z := zh[c.ID] // zero value when the id is missing in ZH
		aliases := []string{c.Name, c.EnName, z.Name, z.LogName, z.EnName}
		key := strings.ToLower(name)
		if j, ok := pos[key]; ok {
			chars[j].Aliases = cleanAliases(chars[j].Name, append(chars[j].Aliases, aliases...))
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("punishing gray raven: no characters found")
	}
	if err := SaveCommit(p.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Punishing Gray Raven] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
