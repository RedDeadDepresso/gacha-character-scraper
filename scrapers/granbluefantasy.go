package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
)

const (
	gbfRepo   = "MizaGBF/GBFAL"
	gbfBranch = "main"
	gbfURL    = "https://raw.githubusercontent.com/MizaGBF/GBFAL/main/json/data.json"
)

var (
	gbfDateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	gbfRomajiRe = regexp.MustCompile(`^[\p{Latin}'.\-]+$`)
)

// GranblueFantasy reads playable characters from MizaGBF/GBFAL's json/data.json.
type GranblueFantasy struct{}

func (GranblueFantasy) Name() string { return "Granblue Fantasy" }

// gbfLookup is a parsed entry of data.json's "lookup" object, a string such as
//
//	/w Jeanne_d'Arc_(Summer) /e wind /a ssr /n jeanne d'arc /c summer ... /y ジャンヌダルク megumi han 潘めぐみ 2018-03-16
type gbfLookup struct {
	wiki   string // wiki page, e.g. "Jeanne_d'Arc_(Summer)"
	nameJP string
	date   string // release date, YYYY-MM-DD
}

func parseGBFLookup(s string) gbfLookup {
	// Split into "/x" markers followed by their words.
	fields := map[string][]string{}
	cur := ""
	for _, tok := range strings.Fields(s) {
		if len(tok) > 1 && tok[0] == '/' {
			cur = tok[1:]
			if _, ok := fields[cur]; !ok {
				fields[cur] = []string{}
			}
			continue
		}
		if cur != "" {
			fields[cur] = append(fields[cur], tok)
		}
	}
	var l gbfLookup
	if w := fields["w"]; len(w) > 0 {
		l.wiki = w[0]
	}
	// /y is "<Name JP> <Voice Actor> <Voice Actor JP> <Release Date>". The
	// Japanese name is the run of words before the romanised voice actor
	// (or the date, when there is no voice actor).
	var jp []string
	for _, w := range fields["y"] {
		if gbfDateRe.MatchString(w) {
			l.date = w
			break
		}
		if gbfRomajiRe.MatchString(w) {
			break
		}
		jp = append(jp, w)
	}
	// The date may sit after the voice actors; find it if not seen yet.
	if l.date == "" {
		for _, w := range fields["y"] {
			if gbfDateRe.MatchString(w) {
				l.date = w
			}
		}
	}
	l.nameJP = strings.Join(jp, " ")
	return l
}

// Scrape skips everything when the GBFAL commit hasn't changed since the last
// run (cached in config/commits.json). Every playable character id (the
// "characters" object) is one entry, named after its wiki page with
// underscores turned into spaces, e.g. "Jeanne d'Arc (Summer)". The Japanese
// name comes from the lookup string. Entries without a wiki page (trial and cut content) are
// skipped. The list is newest-first by release date, then by id.
func (g GranblueFantasy) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(gbfRepo, gbfBranch)
	if err != nil {
		return nil, err
	}
	if upToDate(g.Name(), sha, existing) {
		log.Printf("[Granblue Fantasy] GBFAL unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(gbfURL)
	if err != nil {
		return nil, err
	}
	var data struct {
		Version    int                        `json:"version"`
		Characters map[string]json.RawMessage `json:"characters"`
		Lookup     map[string]string          `json:"lookup"`
	}
	err = json.NewDecoder(body).Decode(&data)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse data.json: %w", err)
	}
	if data.Version != 5 {
		log.Printf("[Granblue Fantasy] warning: data.json version is %d, expected 5", data.Version)
	}

	type entry struct {
		id string
		gbfLookup
	}
	var list []entry
	for id := range data.Characters {
		l := parseGBFLookup(data.Lookup[id])
		if l.wiki == "" {
			continue
		}
		list = append(list, entry{id, l})
	}
	// Newest first. Ids are fixed-width, so comparing them as strings is fine.
	sort.Slice(list, func(i, j int) bool {
		if list[i].date != list[j].date {
			return list[i].date > list[j].date
		}
		return list[i].id > list[j].id
	})

	chars := make([]Character, 0, len(list))
	pos := map[string]int{}
	for _, e := range list {
		name := strings.Join(strings.Fields(strings.ReplaceAll(e.wiki, "_", " ")), " ")
		b := newNameBuilder().Name(LangEN, name)
		if jp := e.nameJP; jp != "" && !strings.ContainsAny(jp, "?？") {
			b.Name(LangJA, jp)
		}
		c := b.Character()
		key := strings.ToLower(c.Name[LangEN])
		if p, ok := pos[key]; ok {
			chars[p] = builderOf(chars[p]).Merge(c).Character()
			continue
		}
		pos[key] = len(chars)
		chars = append(chars, c)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("granblue fantasy: no characters found")
	}
	if err := SaveCommit(g.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Granblue Fantasy] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}
