package scrapers

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const gfl2URL = "https://raw.githubusercontent.com/Infernal-Crack-LED/gfl2-team-builder/refs/heads/main/data/dolls.json"

// gfl2Source is one official "all roles" page. It lists every doll with its
// gunDataId and its name in one language; the English names come from gfl2URL.
// A page marked browser is built by JavaScript, so it is rendered in a
// headless browser (gfl2RenderPage) instead of downloaded.
type gfl2Source struct {
	lang, url string
	browser   bool
}

// gfl2NameSelector matches the name of a doll on an "all roles" page.
const gfl2NameSelector = ".allroles-roles-name-name-div"

// gfl2Langs is a variable so tests can point it at a fake server.
var gfl2Langs = []gfl2Source{
	{LangJA, "https://gf2.haoplay.com/jp/roleinfo/allroles.html", false},
	{LangKO, "https://gf2.haoplay.com/kr/roleinfo/allroles.html", false},
	{LangTW, "https://gf2.haoplay.com/tw/roleinfo/allroles.html", false},
	{LangZH, "https://gf2.sunborngame.com/main/allRoles", true},
}

// GirlsFrontline2 reads dolls from the gfl2-team-builder open game data (English
// name and gunDataId) and adds the Japanese, Korean, Traditional and Simplified
// Chinese names from the official "all roles" pages, matched by gunDataId. A
// doll that is on those pages but has no English name yet is kept too, under
// its first available name. The Simplified Chinese page is built by
// JavaScript, so it is read with Playwright (headless Chromium, see
// gfl2browser.go).
// The name has no apostrophe or colon so it is safe as a file name.
type GirlsFrontline2 struct{}

func (GirlsFrontline2) Name() string { return "Girls Frontline 2" }

type gfl2Doll struct {
	Name      string `json:"name"`
	GunDataID int    `json:"gunDataId"`
}

// gfl2Names holds the localized names: language code -> gunDataId -> name.
type gfl2Names map[string]map[int]string

var (
	gfl2AnchorRe  = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	gfl2NameDivRe = regexp.MustCompile(`(?is)<div\b([^>]*\ballroles-roles-name-name-div\b[^>]*)>(.*?)</div>`)
	gfl2ImgRe     = regexp.MustCompile(`(?is)<img\b[^>]*?\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	gfl2ClassRe   = regexp.MustCompile(`(?is)\bclass\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	gfl2BrRe      = regexp.MustCompile(`(?i)<br\s*/?>`)
	gfl2TagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	gfl2IDClassRe = regexp.MustCompile(`^allroles-roles-name-name-div-(\d+)$`)
)

// classTokens returns the CSS classes of an HTML tag's attribute text.
func classTokens(attrs string) []string {
	m := gfl2ClassRe.FindStringSubmatch(attrs)
	if m == nil {
		return nil
	}
	return strings.Fields(m[1] + m[2])
}

func hasClass(attrs, class string) bool {
	for _, c := range classTokens(attrs) {
		if c == class {
			return true
		}
	}
	return false
}

func gfl2Text(inner string) string {
	inner = gfl2BrRe.ReplaceAllString(inner, " ")
	return strings.Join(strings.Fields(html.UnescapeString(gfl2TagRe.ReplaceAllString(inner, ""))), " ")
}

// gfl2ImageID reads the gunDataId from an image URL such as ".../role-1013.png":
// the part after the last "-", without the extension.
func gfl2ImageID(src string) (int, bool) {
	if i := strings.IndexAny(src, "?#"); i >= 0 {
		src = src[:i]
	}
	seg := src[strings.LastIndex(src, "-")+1:]
	id, err := strconv.Atoi(strings.TrimSuffix(seg, path.Ext(seg)))
	return id, err == nil && id > 0
}

// parseGFL2Roles reads gunDataId -> name from an "all roles" page. A doll is an
// <a class="allroles-roles-main-div"> holding an <img> whose file name ends in
// the gunDataId and a <div class="allroles-roles-name-name-div"> with the name.
// The Chinese page (read after JavaScript has run) puts the id in the name
// div's class instead (allroles-roles-name-name-div-<id>), so that form is
// read too.
func parseGFL2Roles(page string) map[int]string {
	out := map[int]string{}
	set := func(id int, name string) {
		if _, ok := out[id]; !ok && name != "" {
			out[id] = name
		}
	}
	for _, a := range gfl2AnchorRe.FindAllStringSubmatch(page, -1) {
		if !hasClass(a[1], "allroles-roles-main-div") {
			continue
		}
		img := gfl2ImgRe.FindStringSubmatch(a[2])
		if img == nil {
			continue
		}
		id, ok := gfl2ImageID(img[1] + img[2])
		if !ok {
			continue
		}
		for _, d := range gfl2NameDivRe.FindAllStringSubmatch(a[2], -1) {
			if hasClass(d[1], "allroles-roles-name-name-div") {
				set(id, gfl2Text(d[2]))
				break
			}
		}
	}
	for _, d := range gfl2NameDivRe.FindAllStringSubmatch(page, -1) {
		if !hasClass(d[1], "allroles-roles-name-name-div") {
			continue
		}
		for _, c := range classTokens(d[1]) {
			if m := gfl2IDClassRe.FindStringSubmatch(c); m != nil {
				id, _ := strconv.Atoi(m[1])
				set(id, gfl2Text(d[2]))
			}
		}
	}
	return out
}

// fetchGFL2Page returns the HTML of an "all roles" page, rendered in a browser
// when the page needs JavaScript.
func fetchGFL2Page(src gfl2Source) (string, error) {
	if src.browser {
		return gfl2RenderPage(src.url, gfl2NameSelector)
	}
	body, err := get(src.url)
	if err != nil {
		return "", err
	}
	defer body.Close()
	page, err := io.ReadAll(body)
	return string(page), err
}

// fetchGFL2Names downloads the localized name pages. A language that fails is
// logged and left out; it must not stop the scrape.
func fetchGFL2Names() gfl2Names {
	names := gfl2Names{}
	for _, l := range gfl2Langs {
		page, err := fetchGFL2Page(l)
		if err != nil {
			log.Printf("[Girls Frontline 2] %s names: %v", l.lang, err)
			continue
		}
		m := parseGFL2Roles(page)
		if len(m) == 0 {
			log.Printf("[Girls Frontline 2] %s names: no dolls found on %s (selector changed?)", l.lang, l.url)
			continue
		}
		log.Printf("[Girls Frontline 2] %s names: %d dolls", l.lang, len(m))
		names[l.lang] = m
	}
	return names
}

// gfl2LangOrder is the order in which the non-English names of a saved doll
// are visited, fixed so that aliases come out the same on every run.
var gfl2LangOrder = []string{LangZH, LangTW, LangJA, LangKO, LangRU}

// gfl2Entry is one doll: its gunDataId and its English name, empty when the
// doll is only known from the language pages.
type gfl2Entry struct {
	id  int
	en  string
	pos int // position in dolls.json, to order dolls that share an id
}

// buildGFL2 merges the doll list and the localized names by gunDataId and
// keeps every doll that is in either of them, so a doll without an English
// name is not lost: it is named after its first available language (the
// builder promotes that name to the English slot).
//
// Dolls already saved keep their entry (names and hand-written aliases): a
// language they lack is filled in, and a scraped name that differs from the
// saved one becomes an alias. A saved doll is found again through any of its
// names, not just the English one, since an untranslated doll is saved under
// a foreign name. When such a doll gets its English name, that name replaces
// the placeholder. Dolls that are new go to the front, newest first (a higher
// gunDataId is a later release). It returns the list and how many dolls are new.
func buildGFL2(dolls []gfl2Doll, names gfl2Names, existing []Character) ([]Character, int) {
	localized := func(id int) []string { // the doll's names, in language order
		var out []string
		for _, l := range gfl2Langs {
			if n := names[l.lang][id]; n != "" {
				out = append(out, n)
			}
		}
		return out
	}

	var list []gfl2Entry
	seen := map[int]bool{}
	for i, d := range dolls {
		en := strings.Join(strings.Fields(d.Name), " ")
		if en == "" && (d.GunDataID <= 0 || len(localized(d.GunDataID)) == 0) {
			continue // nothing to call it
		}
		list = append(list, gfl2Entry{d.GunDataID, en, i})
		if d.GunDataID > 0 {
			seen[d.GunDataID] = true
		}
	}
	for _, l := range gfl2Langs { // dolls only the language pages know
		for id := range names[l.lang] {
			if !seen[id] {
				seen[id] = true
				list = append(list, gfl2Entry{id, "", len(dolls) + id})
			}
		}
	}
	sort.SliceStable(list, func(a, b int) bool { // newest first
		if list[a].id != list[b].id {
			return list[a].id > list[b].id
		}
		return list[a].pos > list[b].pos
	})

	// Lowercase name -> doll, English names before localized ones.
	byName := map[string]gfl2Entry{}
	add := func(name string, e gfl2Entry) {
		if k := strings.ToLower(name); k != "" {
			if _, ok := byName[k]; !ok {
				byName[k] = e
			}
		}
	}
	for _, e := range list {
		add(e.en, e)
	}
	for _, e := range list {
		for _, n := range localized(e.id) {
			add(n, e)
		}
	}

	localize := func(b *nameBuilder, id int) {
		for _, l := range gfl2Langs {
			if n := names[l.lang][id]; n != "" {
				b.Name(l.lang, n)
			}
		}
	}
	find := func(c Character) (gfl2Entry, bool) {
		cands := []string{c.Name.EN()}
		for _, l := range gfl2LangOrder {
			cands = append(cands, c.Name[l])
		}
		cands = append(cands, c.Aliases...)
		for _, n := range cands {
			if e, ok := byName[strings.ToLower(strings.Join(strings.Fields(n), " "))]; ok {
				return e, true
			}
		}
		return gfl2Entry{}, false
	}

	// Names already saved, including aliases, so a doll that is known under
	// any of them is not added a second time.
	known := map[string]bool{}
	claimed := map[int]bool{} // gunDataIds of saved dolls
	chars := make([]Character, 0, len(existing)+len(list))
	for _, c := range existing {
		for _, n := range c.Name {
			known[strings.ToLower(n)] = true
		}
		for _, a := range c.Aliases {
			known[strings.ToLower(strings.TrimSpace(a))] = true
		}
		if e, ok := find(c); ok {
			if e.id > 0 {
				claimed[e.id] = true
			}
			legacy := c.legacy
			c = refreshGFL2(c, e, localized(e.id), localize)
			c.legacy = legacy
		}
		chars = append(chars, c)
	}

	// list is newest first, so new dolls keep that order at the front.
	var added []Character
	for _, e := range list {
		if e.id > 0 && claimed[e.id] {
			continue
		}
		keys := []string{e.en}
		if e.en == "" {
			keys = localized(e.id)
		}
		dup := false
		for _, k := range keys {
			dup = dup || known[strings.ToLower(k)]
		}
		if dup {
			continue
		}
		for _, k := range keys {
			known[strings.ToLower(k)] = true
		}
		b := newNameBuilder().Name(LangEN, e.en)
		localize(b, e.id)
		added = append(added, b.Character())
	}
	return append(added, chars...), len(added)
}

// refreshGFL2 fills in the missing names of a saved doll. When the doll has an
// English name now but the saved entry only has a foreign name in the English
// slot (it was saved untranslated), the English name replaces that
// placeholder, which stays searchable as the name of its own language.
func refreshGFL2(c Character, e gfl2Entry, loc []string, localize func(*nameBuilder, int)) Character {
	b := builderOf(c)
	if e.en != "" {
		saved := c.Name[LangEN]
		placeholder := false
		for _, n := range loc {
			placeholder = placeholder || strings.EqualFold(n, saved)
		}
		if placeholder && !strings.EqualFold(saved, e.en) {
			b = newNameBuilder().Name(LangEN, e.en)
			for _, l := range gfl2LangOrder {
				b.Name(l, c.Name[l])
			}
			b.Alias(saved).Alias(c.Aliases...)
		} else {
			b.Name(LangEN, e.en) // a different saved English name keeps its place; this becomes an alias
		}
	}
	localize(b, e.id)
	return b.Character()
}

// Scrape fetches the doll list and the localized names and merges them (see
// buildGFL2). It always visits the language pages, because a doll that has no
// English name yet is only listed there.
func (g GirlsFrontline2) Scrape(existing []Character) ([]Character, error) {
	body, err := get(gfl2URL)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Dolls []gfl2Doll `json:"dolls"`
	}
	err = json.NewDecoder(body).Decode(&raw)
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("parse gfl2 dolls.json: %w", err)
	}

	chars, added := buildGFL2(raw.Dolls, fetchGFL2Names(), existing)
	if len(chars) == 0 {
		return nil, fmt.Errorf("girls frontline 2: no dolls found")
	}
	log.Printf("[Girls Frontline 2] %d dolls, %d new", len(chars), added)
	return chars, nil
}
