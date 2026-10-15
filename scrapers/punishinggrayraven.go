package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

const (
	// pgrRepo is only used to detect changes: the scrape is skipped while its
	// latest commit is the one cached in config/commits.json.
	pgrRepo   = "alt3ri/PGR_Data"
	pgrBranch = "master"
	pgrURL    = "https://api.huaxu.app/servers/%s/characters"
)

// pgrServers maps the huaxu.app server of each language to its KKAFIO code.
// "na" is the English one and gives the main name. Every server lists the same
// ids.
var pgrServers = []struct{ server, lang string }{
	{"na", LangEN},
	{"kr", LangKO},
	{"jp", LangJA},
	{"tw", LangTW},
	{"cn", LangZH},
}

// PunishingGrayRaven reads Constructs from the huaxu.app API.
type PunishingGrayRaven struct{}

func (PunishingGrayRaven) Name() string { return "Punishing Gray Raven" }

type pgrCharacter struct {
	ID            int    `json:"id"`
	CharacterName string `json:"characterName"`
}

// fetchPGR downloads one server's character list, keeping the API order.
func fetchPGR(server string) ([]pgrCharacter, error) {
	body, err := get(fmt.Sprintf(pgrURL, server))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Characters []pgrCharacter `json:"characters"`
		} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("parse %s characters: %w", server, err)
	}
	if resp.Status != "" && resp.Status != "success" {
		return nil, fmt.Errorf("huaxu %s: status %q", server, resp.Status)
	}
	return resp.Data.Characters, nil
}

// buildPGRCharacters turns the per-language lists (language -> API-ordered
// characters) into one entry per English characterName. Only characterName is
// used (frameName is ignored), so every frame of a character (Lucia: Lotus,
// Lucia: Inverse Crown, ...) folds into one entry, placed at the position of
// its oldest frame. The English list gives the order and the main name: it is
// in release order, so the result is that order reversed, newest first. The
// other languages are matched by id.
func buildPGRCharacters(lists map[string][]pgrCharacter) []Character {
	byLang := map[string]map[int]string{} // lang -> id -> characterName
	for lang, list := range lists {
		m := make(map[int]string, len(list))
		for _, c := range list {
			m[c.ID] = c.CharacterName
		}
		byLang[lang] = m
	}

	var oldest []Character
	pos := map[string]int{} // lowercase en name -> index in oldest
	for _, c := range lists[LangEN] {
		name := strings.Join(strings.Fields(c.CharacterName), " ")
		if name == "" {
			continue
		}
		b := newNameBuilder().Name(LangEN, name)
		for _, s := range pgrServers[1:] {
			b.Name(s.lang, byLang[s.lang][c.ID])
		}
		built := b.Character()
		key := strings.ToLower(name)
		if p, ok := pos[key]; ok {
			oldest[p] = builderOf(oldest[p]).Merge(built).Character()
			continue
		}
		pos[key] = len(oldest)
		oldest = append(oldest, built)
	}

	chars := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}
	return chars
}

// Scrape skips everything when the PGR_Data commit hasn't changed since the
// last run (cached in config/commits.json). Otherwise it fetches the character
// list of each server from huaxu.app and rebuilds the list.
func (p PunishingGrayRaven) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(pgrRepo, pgrBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(p.Name(), sha, existing) {
		log.Printf("[Punishing Gray Raven] PGR_Data unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	lists := map[string][]pgrCharacter{}
	for _, s := range pgrServers {
		list, err := fetchPGR(s.server)
		if errors.Is(err, ErrNotFound) && s.server != "na" {
			log.Printf("[Punishing Gray Raven] no %s server data, skipping", s.server)
			continue
		}
		if err != nil {
			return nil, err
		}
		lists[s.lang] = list
	}

	chars := buildPGRCharacters(lists)
	if len(chars) == 0 {
		return nil, fmt.Errorf("punishing gray raven: no characters found")
	}
	if err := SaveCommit(p.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Punishing Gray Raven] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
