package scrapers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
)

const (
	gbfURL = "https://raw.githubusercontent.com/MizaGBF/GBFAL/main/json/data.json"
)

var (
	gbfDateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	gbfRomajiRe = regexp.MustCompile(`^[\p{Latin}'.\-]+$`)
)

// GranblueFantasy reads playable characters from MizaGBF/GBFAL's json/data.json
// and adds their Simplified Chinese names from the BLHXFY name tables
// (data/npc-name-jp.csv and data/npc-name-en.csv, see the end of this file).
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

// Scrape skips everything when the ETags of GBFAL's data.json and of the
// BLHXFY name tables haven't changed since the last run (cached in
// config/etags.json). Every playable character id (the
// "characters" object) is one entry, named after its wiki page with
// underscores turned into spaces, e.g. "Jeanne d'Arc (Summer)". The Japanese
// name comes from the lookup string. Entries without a wiki page (trial and cut content) are
// skipped. The list is newest-first by release date, then by id.
func (g GranblueFantasy) Scrape(existing []Character) ([]Character, error) {
	gbfTag, err := SourceTag(gbfURL)
	if err != nil {
		return nil, err
	}
	// The Chinese names come from the BLHXFY name tables. The cached key is
	// "<GBFAL tag>+<BLHXFY tag>", so a change in either source triggers a
	// rebuild. A table path that does not exist (the CSVs have moved between
	// data/ and data/etc/) has a stable tag of its own, so only the files that
	// exist matter. When BLHXFY cannot be reached the key is just the GBFAL tag:
	// the list is built without Chinese names and rebuilt on the next run that
	// can reach it.
	blhxfyTag, err := SourceTag(blhxfyURLs()...)
	if err != nil {
		log.Printf("[Granblue Fantasy] warning: BLHXFY: %v (no Chinese names)", err)
		blhxfyTag = ""
	}
	key := gbfTag
	if blhxfyTag != "" {
		key = gbfTag + "+" + blhxfyTag
	}
	if upToDate(g.Name(), key, existing) {
		log.Printf("[Granblue Fantasy] GBFAL and BLHXFY unchanged (%s), skipping", key[:7])
		return existing, nil
	}

	var zh *gbfNames
	if blhxfyTag != "" {
		if zh, err = fetchGBFNames(blhxfyRef); err != nil {
			log.Printf("[Granblue Fantasy] warning: BLHXFY name tables: %v (no Chinese names)", err)
			zh, key = nil, gbfTag
		}
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
	translated := 0
	for _, e := range list {
		name := strings.Join(strings.Fields(strings.ReplaceAll(e.wiki, "_", " ")), " ")
		b := newNameBuilder().Name(LangEN, name)
		if jp := e.nameJP; jp != "" && !strings.ContainsAny(jp, "?？") {
			b.Name(LangJA, jp)
		}
		if zh != nil {
			if cn := zh.Chinese(name, e.nameJP); cn != "" {
				b.Name(LangZH, cn)
				translated++
			}
		}
		c := b.Character()
		lk := strings.ToLower(c.Name[LangEN])
		if p, ok := pos[lk]; ok {
			chars[p] = builderOf(chars[p]).Merge(c).Character()
			continue
		}
		pos[lk] = len(chars)
		chars = append(chars, c)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("granblue fantasy: no characters found")
	}
	if err := SaveTag(g.Name(), key); err != nil {
		return nil, err
	}
	if zh != nil {
		log.Printf("[Granblue Fantasy] %d characters, %d with a Chinese name (etag %s)", len(chars), translated, key[:7])
	} else {
		log.Printf("[Granblue Fantasy] %d characters (etag %s)", len(chars), key[:7])
	}
	return chars, nil
}

// ---- Chinese names (BLHXFY name tables) ----

const (
	blhxfyRepo = "BLHXFY-Group/BLHXFY"
	// "HEAD" is the default branch, so the branch name does not matter.
	blhxfyRef = "HEAD"
	blhxfyRaw = "https://raw.githubusercontent.com/" + blhxfyRepo + "/%s/%s"
)

// blhxfyPaths lists where each CSV may live in the repo, in order. The
// project has kept the NPC name files in data/ and the other helper files in
// data/etc/, so both are tried.
var (
	blhxfyENPaths = []string{"data/npc-name-en.csv", "data/etc/npc-name-en.csv"}
	blhxfyJPPaths = []string{"data/npc-name-jp.csv", "data/etc/npc-name-jp.csv"}
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// gbfNames translates Granblue Fantasy character names to Simplified Chinese
// with the name tables of the BLHXFY project (npc-name-jp.csv and
// npc-name-en.csv).
type gbfNames struct {
	jp     map[string]string // Japanese name -> Chinese
	jpNorm map[string]string // normalised Japanese name -> Chinese
	en     map[string]string // lowercase English name -> Chinese
}

// gbfVariantZH translates the "(Summer)" part of a name like "Jeanne d'Arc
// (Summer)". Keys are lowercase. A variant that is not listed here keeps its
// English text, which makes it easy to spot; add it to this table.
var gbfVariantZH = map[string]string{
	"summer":    "泳装",
	"halloween": "万圣节",
	"holiday":   "圣诞",
	"yukata":    "浴衣",
	"valentine": "情人节",
	"formal":    "礼服",
	"light":     "光",
	"dark":      "暗",
	"fire":      "火",
	"water":     "水",
	"earth":     "土",
	"wind":      "风",
}

// gbfVariantDrop lists the parentheses that only mark the rarity or the kind
// of release. They are not part of a name, so they are left out.
var gbfVariantDrop = map[string]bool{
	"ssr": true, "sr": true, "r": true, "event": true, "event ssr": true,
	"promo": true, "collab": true,
}

// normJP removes what differs between the sources: GBFAL writes
// "ジャンヌダルク" where BLHXFY has "ジャンヌ・ダルク".
func normJP(s string) string {
	return strings.NewReplacer("・", "", "＝", "", "=", "", "·", "", " ", "", "　", "").Replace(s)
}

// readNameCSV reads a BLHXFY name table. Both files have a header row with at
// least the columns `name` and `trans` (npc-name-en.csv has more, and starts
// with a byte order mark).
func readNameCSV(r io.Reader) (map[string]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	nameCol, transCol := -1, -1
	for i, h := range rows[0] {
		switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))) {
		case "name":
			nameCol = i
		case "trans":
			transCol = i
		}
	}
	if nameCol < 0 || transCol < 0 {
		return nil, fmt.Errorf("csv has no name/trans columns")
	}
	out := map[string]string{}
	for _, row := range rows[1:] {
		if len(row) <= nameCol || len(row) <= transCol {
			continue
		}
		name := strings.TrimSpace(htmlTagRe.ReplaceAllString(row[nameCol], ""))
		trans := strings.TrimSpace(htmlTagRe.ReplaceAllString(row[transCol], ""))
		if name == "" || trans == "" {
			continue
		}
		if _, ok := out[name]; !ok { // the first row wins
			out[name] = trans
		}
	}
	return out, nil
}

// newGBFNames builds a translator from the two tables.
func newGBFNames(jp, en map[string]string) *gbfNames {
	n := &gbfNames{jp: jp, jpNorm: map[string]string{}, en: map[string]string{}}
	for k, v := range jp {
		if _, ok := n.jpNorm[normJP(k)]; !ok {
			n.jpNorm[normJP(k)] = v
		}
	}
	for k, v := range en {
		if _, ok := n.en[strings.ToLower(k)]; !ok {
			n.en[strings.ToLower(k)] = v
		}
	}
	return n
}

// blhxfyURLs lists every place the two name tables may live, for their ETags.
func blhxfyURLs() []string {
	var urls []string
	for _, p := range append(append([]string{}, blhxfyJPPaths...), blhxfyENPaths...) {
		urls = append(urls, fmt.Sprintf(blhxfyRaw, blhxfyRef, p))
	}
	return urls
}

// fetchGBFNames downloads both tables from the BLHXFY ref (a branch, tag or commit).
func fetchGBFNames(ref string) (*gbfNames, error) {
	fetch := func(paths []string) (map[string]string, error) {
		var lastErr error
		for _, p := range paths {
			body, err := get(fmt.Sprintf(blhxfyRaw, ref, p))
			if err != nil {
				lastErr = err
				continue
			}
			m, err := readNameCSV(body)
			body.Close()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			return m, nil
		}
		return nil, lastErr
	}
	jp, err := fetch(blhxfyJPPaths)
	if err != nil {
		return nil, err
	}
	en, err := fetch(blhxfyENPaths)
	if err != nil {
		return nil, err
	}
	return newGBFNames(jp, en), nil
}

// splitVariant splits "Jeanne d'Arc (Summer)" into "Jeanne d'Arc" and
// "Summer". A name without a trailing "(...)" has an empty variant.
func splitVariant(name string) (base, variant string) {
	name = strings.TrimSpace(name)
	if !strings.HasSuffix(name, ")") {
		return name, ""
	}
	i := strings.LastIndex(name, " (")
	if i <= 0 {
		return name, ""
	}
	return strings.TrimSpace(name[:i]), strings.TrimSpace(name[i+2 : len(name)-1])
}

// Chinese returns the Simplified Chinese name of a character, or "" when the
// tables have no name for it. The base name is looked up by its Japanese name
// first (the most complete table), then by its English name; the variant in
// parentheses is translated with gbfVariantZH.
func (n *gbfNames) Chinese(en, ja string) string {
	base, variant := splitVariant(en)
	zh := ""
	if ja != "" {
		if v, ok := n.jp[ja]; ok {
			zh = v
		} else if v, ok := n.jpNorm[normJP(ja)]; ok {
			zh = v
		}
	}
	if zh == "" {
		if v, ok := n.en[strings.ToLower(base)]; ok {
			zh = v
		} else if v, ok := n.en[strings.ToLower(strings.TrimSpace(en))]; ok {
			return v // the table has this exact variant
		}
	}
	if zh == "" {
		return ""
	}
	key := strings.ToLower(variant)
	if variant == "" || gbfVariantDrop[key] {
		return zh
	}
	// "Water Summer" or "Earth Grand": translate word by word.
	parts := strings.Fields(variant)
	for i, w := range parts {
		if t, ok := gbfVariantZH[strings.ToLower(w)]; ok {
			parts[i] = t
		}
	}
	return zh + "（" + strings.Join(parts, "") + "）"
}
