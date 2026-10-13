// Command merge combines data/*.json into a single gacha_index.json using the
// KKAFIO index format. Gachas earlier in config/games.json rank as more popular.
package main

import (
	"bytes"
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
	out := flag.String("out", "dist/gacha_character_index.json", "output path")
	flag.Parse()

	var cfg config
	mustRead("config/games.json", &cfg)

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
		before := len(list)
		list = mergeAlters(list)
		log.Printf("%s: merged %d alter versions into base characters", game, before-len(list))

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
	// Don't escape &, < and > as \u0026 etc.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(idx); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
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

// alterSeparators mark an alter version of a character: "Ch'en the Dawnstreak"
// or "Amiya (Guardian)" are alters of "Ch'en" / "Amiya".
var alterSeparators = []string{" the ", " ("}

// baseOf returns the lowercase name of the base character if name looks like an
// alter of a character that exists in idx (same series only, since idx only
// holds the current game's characters).
func baseOf(name string, idx map[string]int) (string, bool) {
	for _, sep := range alterSeparators {
		for i := 1; i+len(sep) <= len(name); i++ {
			if strings.EqualFold(name[i:i+len(sep)], sep) {
				base := strings.ToLower(name[:i])
				if _, ok := idx[base]; ok {
					return base, true
				}
			}
		}
	}
	return "", false
}

// mergeAlters folds alter versions into their base character (the older one),
// adding the alter's name and aliases to the base's aliases. Only characters
// within the same list (one game) are merged.
func mergeAlters(list []scraped) []scraped {
	idx := map[string]int{}
	for i, s := range list {
		idx[strings.ToLower(s.Name)] = i
	}
	root := func(i int) int {
		for depth := 0; depth < 10; depth++ {
			base, ok := baseOf(list[i].Name, idx)
			if !ok || idx[base] == i {
				return i
			}
			i = idx[base]
		}
		return i
	}

	out := []scraped{}
	outPos := map[int]int{} // list index of a base -> index in out
	for i, s := range list {
		if root(i) == i {
			outPos[i] = len(out)
			out = append(out, scraped{Name: s.Name, Aliases: union(nil, s.Aliases)})
		}
	}
	for i, s := range list {
		r := root(i)
		if r == i {
			continue
		}
		o := &out[outPos[r]]
		o.Aliases = union(o.Aliases, append([]string{s.Name}, s.Aliases...))
	}
	for i := range out {
		filtered := out[i].Aliases[:0]
		for _, a := range out[i].Aliases {
			if !strings.EqualFold(a, out[i].Name) {
				filtered = append(filtered, a)
			}
		}
		out[i].Aliases = filtered
	}
	return out
}
