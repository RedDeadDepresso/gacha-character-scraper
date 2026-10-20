package bangumi

import (
	"regexp"
	"strings"
	"unicode"
)

// foldWidth maps full-width ASCII (Ａ, １, ！, ...) and the ideographic space to
// their ordinary forms (the part of NFKC that matters for names).
func foldWidth(r rune) rune {
	switch {
	case r >= 0xFF01 && r <= 0xFF5E:
		return r - 0xFEE0
	case r == 0x3000:
		return ' '
	}
	return r
}

// Norm is the case / width / punctuation-insensitive form used for every
// lookup: letters and digits of any script, lower-cased, nothing else (the same
// idea as norm() in agv-character-scraper).
func Norm(s string) string {
	var b strings.Builder
	for _, r := range s {
		r = unicode.ToLower(foldWidth(r))
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	parens    = regexp.MustCompile(`[（(]([^）)]*)[）)]`)
	partSplit = regexp.MustCompile(`[\s,，、·・]+`)
)

// Forms returns every normalised spelling a name is looked up by: the whole
// name, the name without a "(reading)" and the reading itself, and both orders
// of a two-part name ("Last, First", "Last First", "Given·Family"). Forms of
// fewer than two letters are dropped: they would match far too much.
func Forms(raw string) []string {
	set := map[string]bool{}
	add := func(s string) {
		if n := Norm(s); len([]rune(n)) >= 2 {
			set[n] = true
		}
	}
	add(raw)
	bases := []string{raw}
	if parens.MatchString(raw) {
		base := parens.ReplaceAllString(raw, " ")
		bases = append(bases, base)
		add(base)
		for _, m := range parens.FindAllStringSubmatch(raw, -1) {
			add(m[1])
		}
	}
	for _, b := range bases {
		var parts []string
		for _, p := range partSplit.Split(strings.TrimSpace(b), -1) {
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 2 {
			add(parts[1] + parts[0])
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	return out
}

// Scraped is a scraped gacha character: its name in every language and its aliases.
type Scraped struct {
	Names   []string
	Aliases []string
}

// Result is what Match found for one scraped character.
type Result struct {
	Matched  bool
	Collects int  // the Bangumi character's 收藏数, the popularity score
	Collab   bool // matched, but a guest (`type` 3) in the game's subject
}

// Match links scraped characters to the characters of the game's Bangumi
// subject by name. Only that subject's characters are candidates, so a common
// name can only clash with another character of the same game. A candidate
// scores 2 per shared form of the scraped names and 1 per shared form of its
// aliases; the best one wins (then the more popular, then the lower id).
// A matched character is Collab when it is a guest (`type` 3) in the subject.
func (d *Data) Match(subjectID int, chars []Scraped) []Result {
	cands := d.Characters(subjectID)
	byForm := map[string][]*Character{}
	for _, c := range cands {
		seen := map[string]bool{}
		for _, n := range c.Names {
			for _, f := range Forms(n) {
				if !seen[f] {
					seen[f] = true
					byForm[f] = append(byForm[f], c)
				}
			}
		}
	}

	res := make([]Result, len(chars))
	for i, s := range chars {
		score := map[*Character]int{}
		primary := map[string]bool{}
		for _, n := range s.Names {
			for _, f := range Forms(n) {
				if !primary[f] {
					primary[f] = true
					for _, c := range byForm[f] {
						score[c] += 2
					}
				}
			}
		}
		for _, n := range s.Aliases {
			for _, f := range Forms(n) {
				if !primary[f] { // counted once, as a name, when it is one
					primary[f] = true
					for _, c := range byForm[f] {
						score[c]++
					}
				}
			}
		}
		var best *Character
		for c, sc := range score {
			if best == nil || sc > score[best] ||
				(sc == score[best] && (c.Collects > best.Collects ||
					(c.Collects == best.Collects && c.ID < best.ID))) {
				best = c
			}
		}
		if best == nil {
			continue
		}
		res[i] = Result{Matched: true, Collects: best.Collects}
		res[i].Collab = d.IsCollab(subjectID, best)
	}
	return res
}
