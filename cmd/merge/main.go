// Command merge combines data/*.json into a single gacha_index.json using the
// KKAFIO index format. Gachas earlier in config.json rank as more popular.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type config struct {
	Gachas []string `json:"gachas"`
}

type scraped struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type series struct {
	Title   string   `json:"title"`
	Aliases []string `json:"aliases"`
}

type char struct {
	Name    string   `json:"name"`
	Series  []int    `json:"series"`
	Score   int      `json:"score"`
	Aliases []string `json:"aliases"`
}

type index struct {
	Version int      `json:"version"`
	Series  []series `json:"series"`
	Chars   []char   `json:"chars"`
}

func main() {
	out := flag.String("out", "dist/gacha_index.json", "output path")
	flag.Parse()

	var cfg config
	mustRead("config.json", &cfg)

	idx := index{Version: 1, Series: []series{}, Chars: []char{}}
	pos := map[string]int{} // lowercase name -> index in idx.Chars

	for gi, game := range cfg.Gachas {
		path := filepath.Join("data", game+".json")
		if _, err := os.Stat(path); err != nil {
			log.Printf("no data for %s, skipping", game)
			continue
		}
		var list []scraped
		mustRead(path, &list)

		sIdx := len(idx.Series)
		idx.Series = append(idx.Series, series{Title: game, Aliases: []string{}})

		// Game popularity from config order: 100, 90, 80 ... (min 10).
		base := 100 - gi*10
		if base < 10 {
			base = 10
		}
		n := len(list)
		for i, s := range list {
			// Data files are newest-first, so a higher index means an older
			// character; older characters score higher (base down to base/2).
			score := base - (base/2)*(n-1-i)/n
			key := strings.ToLower(s.Name)
			if p, ok := pos[key]; ok {
				c := &idx.Chars[p]
				c.Series = append(c.Series, sIdx)
				if score > c.Score {
					c.Score = score
				}
				c.Aliases = union(c.Aliases, s.Aliases)
				continue
			}
			pos[key] = len(idx.Chars)
			idx.Chars = append(idx.Chars, char{
				Name: s.Name, Series: []int{sIdx}, Score: score, Aliases: union(nil, s.Aliases),
			})
		}
	}

	sort.SliceStable(idx.Chars, func(i, j int) bool { return idx.Chars[i].Score > idx.Chars[j].Score })

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	b, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d series, %d characters", *out, len(idx.Series), len(idx.Chars))
}

func mustRead(path string, v any) {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("%s: %v", path, err)
	}
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	res := []string{}
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	return res
}