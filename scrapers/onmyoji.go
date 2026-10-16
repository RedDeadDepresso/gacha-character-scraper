package scrapers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// onmyojiAPI is the MediaWiki API of the Onmyoji fandom wiki. It is a variable
// so tests can point it at a fake server.
var onmyojiAPI = "https://onmyoji.fandom.com/api.php"

// onmyojiCategories are the rarity categories whose members are the
// shikigami pages. The rarity itself is not kept: the list is ordered by page
// creation date only.
var onmyojiCategories = []string{"SP", "SSR", "SR", "R", "N"}

var (
	onmyojiCNRe = regexp.MustCompile(`(?i)\|\s*name_cn\s*=\s*([^|\n}]+)`)
	onmyojiJPRe = regexp.MustCompile(`(?i)\|\s*name_jp\s*=\s*([^|\n}]+)`)
	onmyojiGLRe = regexp.MustCompile(`(?i)\|\s*name_gl\s*=\s*([^|\n}]+)`)
	onmyojiBrRe = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// Onmyoji reads the shikigami of Onmyoji from the Onmyoji fandom wiki: the
// page title is the English name, and the Chinese, Japanese and global names
// are read from the name_cn, name_jp and name_gl fields of the page.
type Onmyoji struct{}

func (Onmyoji) Name() string { return "Onmyoji" }

// onmyojiResponse is the part of a formatversion=2 MediaWiki answer we use.
type onmyojiResponse struct {
	Continue map[string]string `json:"continue"`
	Query    struct {
		CategoryMembers []struct {
			Title string `json:"title"`
		} `json:"categorymembers"`
		Pages []struct {
			Title     string `json:"title"`
			Revisions []struct {
				Timestamp string `json:"timestamp"`
				Slots     struct {
					Main struct {
						Content string `json:"content"`
					} `json:"main"`
				} `json:"slots"`
			} `json:"revisions"`
		} `json:"pages"`
	} `json:"query"`
}

func onmyojiQuery(params url.Values) (*onmyojiResponse, error) {
	params.Set("action", "query")
	params.Set("format", "json")
	params.Set("formatversion", "2")
	body, err := get(onmyojiAPI + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var out onmyojiResponse
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse onmyoji api: %w", err)
	}
	return &out, nil
}

// fetchOnmyojiTitles lists the pages of every rarity category (main namespace
// only, so sub-categories and talk pages are left out), without duplicates.
func fetchOnmyojiTitles() ([]string, error) {
	var titles []string
	seen := map[string]bool{}
	for _, cat := range onmyojiCategories {
		params := url.Values{
			"list":        {"categorymembers"},
			"cmtitle":     {"Category:" + cat},
			"cmnamespace": {"0"},
			"cmlimit":     {"500"},
		}
		for {
			res, err := onmyojiQuery(params)
			if err != nil {
				return nil, fmt.Errorf("category %s: %w", cat, err)
			}
			for _, m := range res.Query.CategoryMembers {
				if !seen[m.Title] {
					seen[m.Title] = true
					titles = append(titles, m.Title)
				}
			}
			if len(res.Continue) == 0 {
				break
			}
			for k, v := range res.Continue {
				params.Set(k, v)
			}
		}
	}
	return titles, nil
}

// fetchOnmyojiWikitext returns title -> wikitext for the given pages, 50 pages
// per request.
func fetchOnmyojiWikitext(titles []string) (map[string]string, error) {
	out := make(map[string]string, len(titles))
	for i := 0; i < len(titles); i += 50 {
		end := min(i+50, len(titles))
		log.Printf("[Onmyoji] wikitext %d-%d of %d", i+1, end, len(titles))
		res, err := onmyojiQuery(url.Values{
			"prop":    {"revisions"},
			"rvprop":  {"content"},
			"rvslots": {"main"},
			"titles":  {strings.Join(titles[i:end], "|")},
		})
		if err != nil {
			return nil, err
		}
		for _, p := range res.Query.Pages {
			if len(p.Revisions) > 0 {
				out[p.Title] = p.Revisions[0].Slots.Main.Content
			}
		}
	}
	return out, nil
}

// fetchOnmyojiCreated returns the timestamp of the first revision of a page.
// The API only allows rvlimit for a single page, so this is one request per
// page.
func fetchOnmyojiCreated(title string) (string, error) {
	res, err := onmyojiQuery(url.Values{
		"prop":    {"revisions"},
		"rvprop":  {"timestamp"},
		"rvdir":   {"newer"},
		"rvlimit": {"1"},
		"titles":  {title},
	})
	if err != nil {
		return "", err
	}
	for _, p := range res.Query.Pages {
		if len(p.Revisions) > 0 {
			return p.Revisions[0].Timestamp, nil
		}
	}
	return "", nil
}

// onmyojiField returns the first value of a wikitext template field; when the
// value has several lines (separated by <br>) only the first is used.
func onmyojiField(re *regexp.Regexp, wikitext string) string {
	m := re.FindStringSubmatch(wikitext)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(onmyojiBrRe.Split(m[1], 2)[0])
}

// buildOnmyojiCharacter makes one character from a page title and its
// wikitext: the title is the English name, name_cn / name_jp the Chinese and
// Japanese ones, and a name_gl that differs from the title becomes an alias.
func buildOnmyojiCharacter(title, wikitext string) Character {
	b := newNameBuilder().
		Name(LangEN, title).
		Name(LangZH, onmyojiField(onmyojiCNRe, wikitext)).
		Name(LangJA, onmyojiField(onmyojiJPRe, wikitext))
	if gl := onmyojiField(onmyojiGLRe, wikitext); gl != "" && !strings.EqualFold(gl, title) {
		b.Alias(gl)
	}
	return b.Character()
}

// Scrape lists every shikigami page of the rarity categories. Characters that
// are already saved keep their position (their names are refreshed from the
// wiki); pages that are new are put first, newest page first. The creation
// date is only looked up for new pages.
func (o Onmyoji) Scrape(existing []Character) ([]Character, error) {
	titles, err := fetchOnmyojiTitles()
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		return nil, fmt.Errorf("onmyoji: no shikigami found")
	}
	log.Printf("[Onmyoji] %d shikigami pages", len(titles))

	wikitext, err := fetchOnmyojiWikitext(titles)
	if err != nil {
		return nil, err
	}

	known := map[string]bool{} // lowercase en name of saved characters
	for _, c := range existing {
		known[strings.ToLower(c.Name.EN())] = true
	}

	type fresh struct{ title, created string }
	var added []fresh
	for _, t := range titles {
		if known[strings.ToLower(t)] {
			continue
		}
		created, err := fetchOnmyojiCreated(t)
		if err != nil {
			log.Printf("[Onmyoji] creation date of %q: %v", t, err)
		}
		added = append(added, fresh{t, created})
		time.Sleep(200 * time.Millisecond)
	}
	sort.SliceStable(added, func(i, j int) bool { // newest first
		if added[i].created != added[j].created {
			return added[i].created > added[j].created
		}
		return added[i].title < added[j].title
	})

	chars := make([]Character, 0, len(added)+len(existing))
	for _, a := range added {
		chars = append(chars, buildOnmyojiCharacter(a.title, wikitext[a.title]))
	}
	for _, c := range existing {
		if text, ok := wikitext[c.Name.EN()]; ok {
			c = builderOf(buildOnmyojiCharacter(c.Name.EN(), text)).Alias(c.Aliases...).Character()
		}
		chars = append(chars, c)
	}
	log.Printf("[Onmyoji] %d characters (%d new)", len(chars), len(added))
	return chars, nil
}
