// Command main runs every gacha scraper listed in config/series.json and saves the
// results to data/<gacha>.json.
package main

import (
	"log"
	"os"

	"github.com/reddeaddepresso/gacha-character-scraper/scrapers"
)

const dataDir = "data"

// registry lists every available scraper. Add new gachas here.
var registry = []scrapers.Scraper{
	scrapers.Arknights{},
	scrapers.BlueArchive{},
	scrapers.AzurLane{},
	scrapers.UmaMusume{},
	scrapers.ZenlessZoneZero{},
	scrapers.WutheringWaves{},
	scrapers.ArknightsEndfield{},
	scrapers.HonkaiStarRail{},
	scrapers.HonkaiImpact{},
	scrapers.NevernessToEverness{},
	scrapers.Nikke{},
	scrapers.FateGrandOrder{},
	scrapers.GenshinImpact{},
	scrapers.GirlsFrontline2{},
	scrapers.PokemonMasters{},
	scrapers.Hololive{},
	scrapers.GranblueFantasy{},
	scrapers.PunishingGrayRaven{},
	scrapers.Atelier{},
	scrapers.Onmyoji{},
}

func main() {
	series, err := scrapers.LoadSeries()
	if err != nil {
		log.Fatal(err)
	}
	var games []string
	for _, sr := range series {
		games = append(games, sr.Game)
	}

	failed := false
	for _, s := range registry {
		if !contains(games, s.Name()) {
			log.Printf("[%s] not in config/series.json, skipping", s.Name())
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