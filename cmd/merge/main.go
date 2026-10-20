// Command merge combines data/*.json into a single gacha_character_index.json. using the
// KKAFIO index format. Games earlier in config/series.json rank as more popular.
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

	"github.com/reddeaddepresso/gacha-character-scraper/scrapers"
)

// scraped is a character of data/<game>.json (old single-string names load too).
type scraped = scrapers.Character

type series struct {
	Title   scrapers.Names `json:"title"`
	Aliases []string       `json:"aliases"`
}

type char struct {
	Name    scrapers.Names `json:"name"`
	Series  []int          `json:"series"`
	Score   int            `json:"score"`
	Aliases []string       `json:"aliases"`
}

type index struct {
	Version int      `json:"version"`
	Series  []series `json:"series"`
	Chars   []char   `json:"chars"`
}

func main() {
	out := flag.String("out", "dist/gacha_character_index.json", "output path")
	cfg, err := scrapers.LoadSeries()
	if err != nil {
		log.Fatal(err)
	}

	idx := index{Version: 1, Series: []series{}, Chars: []char{}}
	pos := map[string]int{} // lowercase name -> index in idx.Chars

	for gi, sc := range cfg {
		game := sc.Game
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
		title := scrapers.Names{}
		for l, t := range sc.Title {
			if strings.TrimSpace(t) != "" {
				title[l] = strings.TrimSpace(t)
			}
		}
		if title[scrapers.LangEN] == "" {
			title[scrapers.LangEN] = game // folder names stay the game names
		}
		aliases := union(nil, sc.Aliases)
		idx.Series = append(idx.Series, series{Title: title, Aliases: aliases})

		// A game with a bangumi_id uses the scraped Bangumi scores and skips collab
		// characters; otherwise the game's popularity comes from the config order:
		// 100, 90, 80 ... (min 10).
		bgm := sc.BangumiID != 0
		base := 100 - gi*10
		if base < 10 {
			base = 10
		}
		n := len(list)
		for i, s := range list {
			// Data files are newest-first, so a higher index means an older
			// character; older characters score higher (base down to base/2).
			score := base - (base/2)*(n-1-i)/n
			if bgm {
				if s.Collab {
					continue
				}
				score = s.Score // 0 when Bangumi has no such character
			}
			key := strings.ToLower(s.Name.EN())
			if key == "" {
				continue
			}
			if p, ok := pos[key]; ok {
				// Same character in another game: keep the first names, add
				// the other games' different names and aliases as aliases.
				c := &idx.Chars[p]
				c.Series = append(c.Series, sIdx)
				if score > c.Score {
					c.Score = score
				}
				c.Name, c.Aliases = foldNames(c.Name, c.Aliases, s)
				continue
			}
			pos[key] = len(idx.Chars)
			idx.Chars = append(idx.Chars, char{
				Name: copyNames(s.Name), Series: []int{sIdx}, Score: score, Aliases: union(nil, s.Aliases),
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

func copyNames(n scrapers.Names) scrapers.Names {
	out := scrapers.Names{}
	for l, t := range n {
		out[l] = t
	}
	return out
}

// foldNames adds the names of o to names/aliases: a language that has no name
// yet gets o's, a different name in a language that has one becomes an alias,
// and o's aliases are added. Aliases equal to a name are dropped.
func foldNames(names scrapers.Names, aliases []string, o scraped) (scrapers.Names, []string) {
	langs := make([]string, 0, len(o.Name))
	for l := range o.Name {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	extra := append([]string{}, o.Aliases...)
	for _, l := range langs {
		if cur, ok := names[l]; !ok {
			names[l] = o.Name[l]
		} else if !strings.EqualFold(cur, o.Name[l]) {
			extra = append(extra, o.Name[l])
		}
	}
	return names, withoutNames(union(aliases, extra), names)
}

// withoutNames drops aliases that equal one of the names.
func withoutNames(aliases []string, names scrapers.Names) []string {
	skip := map[string]bool{}
	for _, n := range names {
		skip[strings.ToLower(n)] = true
	}
	out := []string{}
	for _, a := range aliases {
		if !skip[strings.ToLower(a)] {
			out = append(out, a)
		}
	}
	return out
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

// alterSeparators mark an alter version of a character: "Ch'en the Dawnstreak",
// "Amiya (Guardian)", "Himeko • Nova", "Nemesis: Gnosis" and "Nemesis·Gnosis"
// are alters of "Ch'en", "Amiya", "Himeko" and "Nemesis". The colon and the
// middle dots (· U+00B7, ・ U+30FB) are matched with or without spaces around
// them.
var alterSeparators = []string{" the ", " (", " • ", ":", "・", "·"}

// baseOf returns the lowercase English name of the base character if name looks
// like an alter of a character that exists in idx (same series only, since idx
// only holds the current game's characters).
func baseOf(name string, idx map[string]int) (string, bool) {
	for _, sep := range alterSeparators {
		for i := 1; i+len(sep) <= len(name); i++ {
			if strings.EqualFold(name[i:i+len(sep)], sep) {
				base := strings.ToLower(strings.TrimSpace(name[:i]))
				if _, ok := idx[base]; ok {
					return base, true
				}
			}
		}
	}
	return "", false
}

// mergeAlters folds alter versions into their base character (the older one),
// adding every name of the alter (all languages) and its aliases to the base's
// aliases. Only characters within the same list (one game) are merged; alters
// are recognised by their English name.
func mergeAlters(list []scraped) []scraped {
	idx := map[string]int{}
	for i, s := range list {
		idx[strings.ToLower(s.Name.EN())] = i
	}
	root := func(i int) int {
		for depth := 0; depth < 10; depth++ {
			base, ok := baseOf(list[i].Name.EN(), idx)
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
			out = append(out, scraped{Name: copyNames(s.Name), Aliases: union(nil, s.Aliases), Score: s.Score, Collab: s.Collab})
		}
	}
	for i, s := range list {
		r := root(i)
		if r == i {
			continue
		}
		o := &out[outPos[r]]
		extra := append([]string{}, s.Aliases...)
		for _, n := range s.Name {
			extra = append(extra, n)
		}
		sort.Strings(extra[len(s.Aliases):]) // map order is random
		o.Aliases = union(o.Aliases, extra)
	}
	for i := range out {
		out[i].Aliases = withoutNames(out[i].Aliases, out[i].Name)
	}
	return out
}
