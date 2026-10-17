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

const (
	gfl2Repo   = "Infernal-Crack-LED/gfl2-team-builder"
	gfl2Branch = "main"
	gfl2URL    = "https://raw.githubusercontent.com/Infernal-Crack-LED/gfl2-team-builder/refs/heads/main/data/dolls.json"
)

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
// Chinese names from the official "all roles" pages, matched by gunDataId. The
// Simplified Chinese page is built by JavaScript, so it is read with Playwright
// (headless Chromium, see gfl2browser.go).
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

// gfl2Complete reports whether every saved doll already has a name in every
// language, i.e. there is nothing left to fill in.
func gfl2Complete(chars []Character) bool {
	for _, c := range chars {
		for _, l := range []string{LangJA, LangKO, LangZH, LangTW} {
			if c.Name[l] == "" {
				return false
			}
		}
	}
	return len(chars) > 0
}

// buildGFL2 merges the doll list and the localized names by gunDataId. Dolls
// already saved keep their entry (names and hand-written aliases): a language
// they lack is filled in, and a scraped name that differs from the saved one
// becomes an alias. Dolls that are new go to the front, newest first (a higher
// gunDataId is a later release). It returns the list and how many dolls are new.
func buildGFL2(dolls []gfl2Doll, names gfl2Names, existing []Character) ([]Character, int) {
	// Newest first; the file position breaks ties (later = newer).
	idx := make([]int, 0, len(dolls))
	for i, d := range dolls {
		if strings.TrimSpace(d.Name) != "" {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		da, db := dolls[idx[a]], dolls[idx[b]]
		if da.GunDataID != db.GunDataID {
			return da.GunDataID > db.GunDataID
		}
		return idx[a] > idx[b]
	})

	byName := map[string]int{} // lowercase en name -> gunDataId
	for _, i := range idx {
		byName[strings.ToLower(strings.Join(strings.Fields(dolls[i].Name), " "))] = dolls[i].GunDataID
	}
	localize := func(b *nameBuilder, id int) {
		for _, l := range gfl2Langs {
			if n := names[l.lang][id]; n != "" {
				b.Name(l.lang, n)
			}
		}
	}

	// Names already saved, including their aliases, so a doll that is known
	// under any of them is not added a second time.
	known := map[string]bool{}
	chars := make([]Character, 0, len(existing)+len(dolls))
	for _, c := range existing {
		for _, n := range c.Name {
			known[strings.ToLower(n)] = true
		}
		for _, a := range c.Aliases {
			known[strings.ToLower(strings.TrimSpace(a))] = true
		}
		id, ok := byName[strings.ToLower(c.Name.EN())]
		for _, a := range c.Aliases {
			if !ok {
				id, ok = byName[strings.ToLower(strings.TrimSpace(a))]
			}
		}
		if ok {
			b := builderOf(c)
			localize(b, id)
			legacy := c.legacy
			c = b.Character()
			c.legacy = legacy
		}
		chars = append(chars, c)
	}

	// idx is newest first, so new dolls keep that order at the front.
	var added []Character
	for _, i := range idx {
		name := strings.Join(strings.Fields(dolls[i].Name), " ")
		k := strings.ToLower(name)
		if known[k] {
			continue
		}
		known[k] = true
		b := newNameBuilder().Name(LangEN, name)
		localize(b, dolls[i].GunDataID)
		added = append(added, b.Character())
	}
	return append(added, chars...), len(added)
}

// Scrape fetches the doll list and the localized names and merges them (see
// buildGFL2). It skips everything when the gfl2-team-builder commit hasn't
// changed since the last run (cached in config/commits.json) and every saved
// doll already has all its names.
func (g GirlsFrontline2) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(gfl2Repo, gfl2Branch)
	if err != nil {
		return nil, err
	}
	// Not upToDate(): a saved list in the old format is converted on load, so
	// there is nothing to rebuild for it.
	if sha == LoadCommit(g.Name()) && gfl2Complete(existing) {
		log.Printf("[Girls Frontline 2] gfl2-team-builder unchanged (%s) and all names present, skipping", sha[:7])
		return existing, nil
	}

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
	if err := SaveCommit(g.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Girls Frontline 2] %d dolls, %d new (commit %s)", len(chars), added, sha[:7])
	return chars, nil
}
