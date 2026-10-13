package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const umaURL = "https://umapyoi.net/api/v1/character/info"

// UmaMusume reads the character list from umapyoi.net.
type UmaMusume struct{}

func (UmaMusume) Name() string { return "Uma Musume Pretty Derby" }

type umaCharacter struct {
	NameEN *string `json:"name_en"`
	NameJP *string `json:"name_jp"`
	GameID *int    `json:"game_id"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// Scrape fetches every character in one request and returns them newest-first
// (lower game_id = older). Entries without a game_id are kept and treated as
// the newest. The whole list is rebuilt each run.
func (u UmaMusume) Scrape(_ []Character) ([]Character, error) {
	body, err := get(umaURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var all []umaCharacter
	if err := json.NewDecoder(body).Decode(&all); err != nil {
		return nil, fmt.Errorf("parse uma musume: %w", err)
	}

	type entry struct {
		name, jp string
		id       int
	}
	best := map[string]entry{} // lowercase name -> lowest game_id
	for _, c := range all {
		// No game_id: age unknown, so treat as newest (lowest score).
		id := 1 << 30
		if c.GameID != nil {
			id = *c.GameID
		}
		name, jp := deref(c.NameEN), deref(c.NameJP)
		if name == "" {
			name = jp // no English name: fall back to the Japanese one
		}
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if cur, ok := best[key]; !ok || id < cur.id {
			best[key] = entry{name: name, jp: jp, id: id}
		}
	}

	list := make([]entry, 0, len(best))
	for _, e := range best {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].id != list[j].id {
			return list[i].id > list[j].id // newest first
		}
		// Same id (e.g. entries without a game_id): sort by name so the order
		// is stable between runs (the map above iterates in random order).
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})

	chars := make([]Character, 0, len(list))
	for _, e := range list {
		c := Character{Name: e.name, Aliases: []string{}}
		if e.jp != "" && !strings.EqualFold(e.jp, e.name) {
			c.Aliases = append(c.Aliases, e.jp)
		}
		chars = append(chars, c)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("uma musume: no characters found")
	}
	log.Printf("[Uma Musume] %d characters", len(chars))
	return chars, nil
}