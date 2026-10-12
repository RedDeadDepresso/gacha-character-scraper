package scrapers

import (
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	arknightsBase    = "https://arknights.wiki.gg"
	arknightsListURL = arknightsBase + "/wiki/Operator/List"
	requestDelay     = 500 * time.Millisecond
)

// Arknights scrapes https://arknights.wiki.gg.
type Arknights struct{}

func (Arknights) Name() string { return "Arknights" }

type operatorLink struct {
	Name string
	URL  string
}

// Scrape walks the operator list in page order. It stops as soon as it reaches
// the first character already saved in data/Arknights.json, so the list page is
// assumed to be ordered newest-first. New characters are prepended to existing.
func (a Arknights) Scrape(existing []Character) ([]Character, error) {
	links, err := a.operatorLinks()
	if err != nil {
		return nil, err
	}
	log.Printf("[Arknights] %d operators on list page", len(links))

	var fresh []Character
	for _, l := range links {
		if len(existing) > 0 && strings.EqualFold(l.Name, existing[0].Name) {
			log.Printf("[Arknights] reached known operator %q, stopping", l.Name)
			break
		}
		time.Sleep(requestDelay)
		c, err := a.operatorPage(l)
		if err != nil {
			log.Printf("[Arknights] skip %s: %v", l.Name, err)
			continue
		}
		log.Printf("[Arknights] + %s %v", c.Name, c.Aliases)
		fresh = append(fresh, c)
	}
	return append(fresh, existing...), nil
}

// operatorLinks reads the name link from each row of the operator list table.
func (Arknights) operatorLinks() ([]operatorLink, error) {
	body, err := get(arknightsListURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	var links []operatorLink
	seen := map[string]bool{}
	doc.Find("table tr").Each(func(_ int, row *goquery.Selection) {
		// First anchor in the row with visible text that points to a wiki page.
		row.Find("a[href^='/wiki/']").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			href, _ := s.Attr("href")
			if text == "" || strings.Contains(href, ":") || seen[href] {
				return true // keep looking in this row
			}
			seen[href] = true
			links = append(links, operatorLink{Name: text, URL: arknightsBase + href})
			return false
		})
	})
	return links, nil
}

// nameLangs are the lang attributes that carry the operator's names in other
// languages, e.g. <div lang="ja">怒潮ズィマー</div>.
var nameLangs = map[string]bool{
	"zh-hans": true, "zh-hant": true, "zh": true, "zh-cn": true, "zh-tw": true,
	"ja": true, "ko": true,
}

// operatorPage collects the names in other languages from the page's
// <div lang="..."> elements. Only the first div per language is used, since
// later ones (voice lines, quotes, ...) are not names.
func (Arknights) operatorPage(l operatorLink) (Character, error) {
	body, err := get(l.URL)
	if err != nil {
		return Character{}, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return Character{}, err
	}

	c := Character{Name: l.Name, Aliases: []string{}}
	seenLang := map[string]bool{}
	seenName := map[string]bool{strings.ToLower(l.Name): true}
	doc.Find("div[lang]").Each(func(_ int, d *goquery.Selection) {
		lang, _ := d.Attr("lang")
		lang = strings.ToLower(strings.TrimSpace(lang))
		if !nameLangs[lang] || seenLang[lang] {
			return
		}
		name := strings.TrimSpace(d.Text())
		if name == "" || seenName[strings.ToLower(name)] {
			return
		}
		seenLang[lang] = true
		seenName[strings.ToLower(name)] = true
		c.Aliases = append(c.Aliases, name)
	})
	return c, nil
}
