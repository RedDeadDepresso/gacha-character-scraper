package scrapers

import (
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
)

// honkaiImpactURL is the character table of the Japanese Honkai Impact 3rd
// wiki (wikiwiki.jp). It is a variable so tests can point it at a fake server.
var honkaiImpactURL = "https://wikiwiki.jp/houkai/%E3%82%AD%E3%83%A3%E3%83%A9%E3%82%AF%E3%82%BF%E3%83%BC"

var (
	hiTbodyRe = regexp.MustCompile(`(?is)<tbody\b[^>]*>(.*?)</tbody>`)
	hiRowRe   = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	hiCellRe  = regexp.MustCompile(`(?is)<td\b[^>]*>(.*?)</td>`)
)

// HonkaiImpact reads the characters of Honkai Impact 3rd from the character
// table of the Japanese wiki: every row of the table body with more than three
// cells is a character, whose first three cells are the Japanese, English and
// Simplified Chinese names. The wiki lists the oldest characters first.
type HonkaiImpact struct{}

func (HonkaiImpact) Name() string { return "Honkai Impact 3rd" }

// hiRow is one table row: the Japanese, English and Chinese names.
type hiRow struct{ ja, en, zh string }

// parseHonkaiImpactRows returns the character rows of the page in page order.
// It looks at the rows of <tbody> (or at every row when the page has no
// <tbody>) that have more than three <td> cells, like the CSS selectors
// "tbody > tr" and "td" do.
func parseHonkaiImpactRows(page string) []hiRow {
	scope := []string{page}
	if bodies := hiTbodyRe.FindAllStringSubmatch(page, -1); len(bodies) > 0 {
		scope = scope[:0]
		for _, b := range bodies {
			scope = append(scope, b[1])
		}
	}
	var rows []hiRow
	for _, s := range scope {
		for _, tr := range hiRowRe.FindAllStringSubmatch(s, -1) {
			cells := hiCellRe.FindAllStringSubmatch(tr[1], -1)
			if len(cells) <= 3 {
				continue
			}
			rows = append(rows, hiRow{
				// The wiki puts a "?" in some Japanese names; it is not part of the name.
				ja: strings.ReplaceAll(gfl2Text(cells[0][1]), "?", ""),
				en: gfl2Text(cells[1][1]),
				zh: gfl2Text(cells[2][1]),
			})
		}
	}
	return rows
}

// buildHonkaiImpact makes the characters, newest first (the page lists the
// oldest first). A row that repeats a character already listed is merged into
// the earlier row, so the character keeps its first position.
func buildHonkaiImpact(rows []hiRow) []Character {
	var oldest []Character
	pos := map[string]int{} // lowercase main name -> index in oldest
	for _, r := range rows {
		c := newNameBuilder().Name(LangEN, r.en).Name(LangJA, r.ja).Name(LangZH, r.zh).Character()
		key := strings.ToLower(c.Name.EN())
		if key == "" {
			continue // an empty row
		}
		if p, ok := pos[key]; ok {
			oldest[p] = mergeCharacters(oldest[p], c)
			continue
		}
		pos[key] = len(oldest)
		oldest = append(oldest, c)
	}
	chars := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}
	return chars
}

// Scrape downloads the wiki table and rebuilds the whole list.
func (HonkaiImpact) Scrape(existing []Character) ([]Character, error) {
	body, err := get(honkaiImpactURL)
	if err != nil {
		return nil, err
	}
	page, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	rows := parseHonkaiImpactRows(string(page))
	chars := buildHonkaiImpact(rows)
	if len(chars) == 0 {
		return nil, fmt.Errorf("honkai impact 3rd: no characters found in the wiki table (did the page layout change?)")
	}
	log.Printf("[Honkai Impact 3rd] %d characters from %d rows", len(chars), len(rows))
	return chars, nil
}
