// Command main runs every gacha scraper listed in config/series.json and saves the
// results to data/<gacha>.json. Every character also gets its Bangumi popularity as a
// score and is flagged when it is a collab character: the Bangumi dump is downloaded
// into bangumi_dump/ (see package bangumi; -no-bangumi skips it).
package main

import (
	"flag"
	"log"
	"os"

	"github.com/reddeaddepresso/gacha-character-scraper/bangumi"
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
	scrapers.GranblueFantasy{},
	scrapers.PunishingGrayRaven{},
	scrapers.Onmyoji{},
}

func main() {
	bangumiDir := flag.String("bangumi-dir", "bangumi_dump", "where the Bangumi dump is downloaded to (git-ignored)")
	noBangumi := flag.Bool("no-bangumi", false, "do not download the Bangumi dump: scores and collab flags are not updated")
	flag.Parse()

	series, err := scrapers.LoadSeries()
	if err != nil {
		log.Fatal(err)
	}
	var games []string
	var ids []int
	byGame := map[string]scrapers.Series{}
	for _, sr := range series {
		games = append(games, sr.Game)
		ids = append(ids, sr.BangumiID)
		byGame[sr.Game] = sr
	}

	var bg *bangumi.Data
	if !*noBangumi {
		if err := bangumi.Update(*bangumiDir); err != nil {
			log.Fatalf("bangumi: %v (use -no-bangumi to skip it)", err)
		}
		if bg, err = bangumi.Load(*bangumiDir, ids); err != nil {
			log.Fatalf("bangumi: %v", err)
		}
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
		scrapers.ApplyBangumi(bg, byGame[s.Name()], chars)
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