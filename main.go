// Command main runs every gacha scraper listed in config/games.json and saves the
// results to data/<gacha>.json.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/reddeaddepresso/gacha-character-scraper/scrapers"
)

const dataDir = "data"

type config struct {
	Gachas []string `json:"gachas"`
}

// registry lists every available scraper. Add new gachas here.
var registry = []scrapers.Scraper{
	scrapers.Arknights{},
	scrapers.BlueArchive{},
	scrapers.AzurLane{},
	scrapers.UmaMusume{},
}

func main() {
	b, err := os.ReadFile(filepath.Join(scrapers.ConfigDir, "games.json"))
	if err != nil {
		log.Fatal(err)
	}
	var cfg config
	if err := json.Unmarshal(b, &cfg); err != nil {
		log.Fatal(err)
	}

	failed := false
	for _, s := range registry {
		if !contains(cfg.Gachas, s.Name()) {
			log.Printf("[%s] not in config/games.json, skipping", s.Name())
			continue
		}
		existing, err := scrapers.LoadData(dataDir, s.Name())
		if err != nil {
			log.Printf("[%s] load: %v", s.Name(), err)
			failed = true
			continue
		}
		chars, err := s.Scrape(existing)
		if err != nil {
			log.Printf("[%s] scrape: %v", s.Name(), err)
			failed = true
			continue
		}
		if err := scrapers.SaveData(dataDir, s.Name(), chars); err != nil {
			log.Printf("[%s] save: %v", s.Name(), err)
			failed = true
			continue
		}
		log.Printf("[%s] saved %d characters", s.Name(), len(chars))
	}
	if failed {
		os.Exit(1)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
