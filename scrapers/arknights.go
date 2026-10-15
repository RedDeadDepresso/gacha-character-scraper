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
	arknightsRepo   = "VanAstraea/novena"
	arknightsBranch = "main"
	arknightsURL    = "https://vanastraea.github.io/novena/data/v1/%s/operators.json"
)

// Servers whose operators.json gives the aliases, in alias order. The English
// file ("en") gives the main name.
var arknightsServers = []string{"jp", "kr", "cn"}

// Arknights reads operators from Novena's static data files.
type Arknights struct{}

func (Arknights) Name() string { return "Arknights" }

type arknightsOp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	CN   string `json:"cn"` // Chinese name, present in the English file
}

// fetchArknightsOps downloads <server>/operators.json ({"ops": [...]}).
func fetchArknightsOps(server string) ([]arknightsOp, error) {
	body, err := get(fmt.Sprintf(arknightsURL, server))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var data struct {
		Ops []arknightsOp `json:"ops"`
	}
	if err := json.NewDecoder(body).Decode(&data); err != nil {
		return nil, fmt.Errorf("parse %s operators.json: %w", server, err)
	}
	return data.Ops, nil
}

// arknightsNumber returns the number in an id like "char_4226_veen" (4226).
func arknightsNumber(id string) int {
	parts := strings.Split(id, "_")
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

// buildArknightsCharacters merges the per-server operator lists by id. The
// name is the English one (an operator not in the English file is skipped) and
// the aliases are the Japanese, Korean and Chinese names. Newest first: a lower
// id number is an older operator.
func buildArknightsCharacters(en []arknightsOp, others map[string][]arknightsOp) []Character {
	byServer := map[string]map[string]string{} // server -> id -> name
	for server, ops := range others {
		m := make(map[string]string, len(ops))
		for _, op := range ops {
			m[op.ID] = strings.TrimSpace(op.Name)
		}
		byServer[server] = m
	}

	list := make([]arknightsOp, 0, len(en))
	for _, op := range en {
		if strings.TrimSpace(op.Name) != "" {
			list = append(list, op)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := arknightsNumber(list[i].ID), arknightsNumber(list[j].ID)
		if a != b {
			return a > b
		}
		return list[i].ID > list[j].ID
	})

	chars := make([]Character, 0, len(list))
	pos := map[string]int{}
	for _, op := range list {
		name := strings.Join(strings.Fields(op.Name), " ")
		var aliases []string
		for _, server := range arknightsServers {
			aliases = append(aliases, byServer[server][op.ID])
		}
		aliases = append(aliases, op.CN) // fallback when the cn file lacks it
		key := strings.ToLower(name)
		if p, ok := pos[key]; ok { // same name under another id: fold into the newer one
			chars[p].Aliases = cleanAliases(chars[p].Name, append(chars[p].Aliases, aliases...))
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	return chars
}

// Scrape skips everything when the Novena commit hasn't changed since the last
// run (cached in config/commits.json). Otherwise the whole list is rebuilt from
// the en, jp, kr and cn operators.json files.
func (a Arknights) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(arknightsRepo, arknightsBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(a.Name()) {
		log.Printf("[Arknights] Novena unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	en, err := fetchArknightsOps("en")
	if err != nil {
		return nil, err
	}
	others := map[string][]arknightsOp{}
	for _, server := range arknightsServers {
		if others[server], err = fetchArknightsOps(server); err != nil {
			return nil, err
		}
	}

	chars := buildArknightsCharacters(en, others)
	if len(chars) == 0 {
		return nil, fmt.Errorf("arknights: no operators found")
	}
	if err := SaveCommit(a.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Arknights] %d operators (commit %s)", len(chars), sha[:7])
	return chars, nil
}
