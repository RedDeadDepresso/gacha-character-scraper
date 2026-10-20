package scrapers

import (
	"log"
	"sort"
	"strings"

	"github.com/reddeaddepresso/gacha-character-scraper/bangumi"
)

// toBangumi gives the names and aliases of the characters to bangumi.Data.Match.
func toBangumi(chars []Character) []bangumi.Scraped {
	out := make([]bangumi.Scraped, len(chars))
	for i, c := range chars {
		langs := make([]string, 0, len(c.Name))
		for l := range c.Name {
			langs = append(langs, l)
		}
		sort.Strings(langs)
		for _, l := range langs {
			out[i].Names = append(out[i].Names, c.Name[l])
		}
		out[i].Aliases = c.Aliases
	}
	return out
}

// ApplyBangumi sets Score (the Bangumi popularity, 0 when Bangumi has no such
// character) and Collab of every character of a game, from the characters of
// the game's Bangumi subject. It does nothing for a game without a bangumi_id.
// The result is logged: the collab characters and the first ones not matched.
func ApplyBangumi(bg *bangumi.Data, sc Series, chars []Character) {
	if bg == nil || sc.BangumiID == 0 || len(chars) == 0 {
		return
	}
	if len(bg.Characters(sc.BangumiID)) == 0 {
		name, typ, ok := bg.Subject(sc.BangumiID)
		log.Printf("WARNING [%s] bangumi_id %d has no characters (subject %q, type %d, in dump: %v): check it in config/series.json",
			sc.Game, sc.BangumiID, name, typ, ok)
	}
	res := bg.Match(sc.BangumiID, toBangumi(chars))
	var collab, unmatched []string
	matched := 0
	for i, r := range res {
		chars[i].Score, chars[i].Collab = r.Collects, r.Collab
		switch {
		case r.Collab:
			matched++
			collab = append(collab, chars[i].Name.EN())
		case r.Matched:
			matched++
		default:
			unmatched = append(unmatched, chars[i].Name.EN())
		}
	}
	log.Printf("[%s] bangumi: matched %d, %d collab, %d without a match (score 0)",
		sc.Game, matched, len(collab), len(unmatched))
	for _, c := range collab {
		log.Printf("[%s]   collab (guest): %s", sc.Game, c)
	}
	if len(unmatched) > 20 {
		unmatched = append(unmatched[:20], "...")
	}
	if len(unmatched) > 0 {
		log.Printf("[%s]   no match: %s", sc.Game, strings.Join(unmatched, "; "))
	}
}
