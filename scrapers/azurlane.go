package scrapers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	azurLaneURL  = "https://raw.githubusercontent.com/Traneptora/azur-lane-data/refs/heads/main/shiplist_%d.json"
	azurRepo     = "Traneptora/azur-lane-data"
	azurBranch   = "main"
	azurLaneMax  = 1000 // safety cap on the number of shiplist files
	azurLaneWait = 200 * time.Millisecond
)

// AzurLane reads the paginated shiplist_<x>.json files from azur-lane-data.
type AzurLane struct{}

func (AzurLane) Name() string { return "Azur Lane" }

type azurShip struct {
	Name   string `json:"Name"`
	CNName string `json:"CNName"`
	JPName string `json:"JPName"`
	KRName string `json:"KRName"`
	ShipID string `json:"ShipID"`
}

type azurPage struct {
	CargoQuery []struct {
		Title azurShip `json:"title"`
	} `json:"cargoquery"`
}

// Scrape downloads shiplist_0.json, shiplist_1.json, ... until a 404, then
// returns the ships newest-first (lower ShipID = older). It skips everything
// when the repo's latest commit matches the one cached in config/commits.json.
func (a AzurLane) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(azurRepo, azurBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(a.Name(), sha, existing) {
		log.Printf("[Azur Lane] azur-lane-data unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	var ships []azurShip
	for x := 0; x < azurLaneMax; x++ {
		page, err := fetchAzurPage(x)
		if errors.Is(err, ErrNotFound) {
			log.Printf("[Azur Lane] shiplist_%d.json is 404, done", x)
			break
		}
		if err != nil {
			return nil, err
		}
		for _, e := range page.CargoQuery {
			ships = append(ships, e.Title)
		}
		time.Sleep(azurLaneWait)
	}
	if len(ships) == 0 {
		return nil, fmt.Errorf("azur lane: no ships found")
	}

	// Keep the lowest ShipID per name. Non-numeric IDs (collabs, plans, ...)
	// sort as newest.
	best := map[string]azurShip{}
	for _, s := range ships {
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" {
			continue
		}
		key := strings.ToLower(s.Name)
		if cur, ok := best[key]; !ok || shipOrder(s.ShipID) < shipOrder(cur.ShipID) {
			best[key] = s
		}
	}
	list := make([]azurShip, 0, len(best))
	for _, s := range best {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		oi, oj := shipOrder(list[i].ShipID), shipOrder(list[j].ShipID)
		if oi != oj {
			return oi > oj // newest first
		}
		return list[i].Name < list[j].Name
	})

	chars := make([]Character, 0, len(list))
	for _, s := range list {
		chars = append(chars, newNameBuilder().
			Name(LangEN, s.Name).
			Name(LangZH, s.CNName).
			Name(LangJA, s.JPName).
			Name(LangKO, s.KRName).
			Character())
	}
	if err := SaveCommit(a.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Azur Lane] %d ships (commit %s)", len(chars), sha[:7])
	return chars, nil
}

func fetchAzurPage(x int) (azurPage, error) {
	var page azurPage
	body, err := get(fmt.Sprintf(azurLaneURL, x))
	if err != nil {
		return page, err
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(&page); err != nil {
		return page, fmt.Errorf("parse shiplist_%d.json: %w", x, err)
	}
	return page, nil
}

// shipOrder converts a ShipID to a sortable number; non-numeric IDs count as
// newest.
func shipOrder(id string) int {
	n, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil {
		return 1 << 30
	}
	return n
}
