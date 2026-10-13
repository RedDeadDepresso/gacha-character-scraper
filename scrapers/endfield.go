package scrapers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
)

const (
	endfieldRepo   = "ivaqis/arknights-tracker"
	endfieldBranch = "main"
	endfieldChars  = "https://raw.githubusercontent.com/ivaqis/arknights-tracker/refs/heads/main/arknights-tracker/src/lib/data/characters.js"
	endfieldLocale = "https://raw.githubusercontent.com/ivaqis/arknights-tracker/refs/heads/main/arknights-tracker/src/lib/locales/%s.json"
)

// Locale files used for aliases, in alias order.
var endfieldLangs = []string{"zhcn", "zhtw", "ja", "ko", "ru"}

// ArknightsEndfield reads characters from the arknights-tracker repo. The name
// has no colon so it is safe as a folder / file name.
type ArknightsEndfield struct{}

func (ArknightsEndfield) Name() string { return "Arknights Endfield" }

// Scrape skips everything when the arknights-tracker commit hasn't changed
// since the last run (cached in config/commits.json). characters.js lists
// characters oldest to newest; the result is stored newest-first.
func (e ArknightsEndfield) Scrape(existing []Character) ([]Character, error) {
	sha, err := LatestCommit(endfieldRepo, endfieldBranch)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && sha == LoadCommit(e.Name()) {
		log.Printf("[Arknights Endfield] arknights-tracker unchanged (%s), skipping", sha[:7])
		return existing, nil
	}

	body, err := get(endfieldChars)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	entries := parseJSObjects(stripCharactersJS(string(raw)))
	if len(entries) == 0 {
		return nil, fmt.Errorf("arknights endfield: no characters parsed from characters.js")
	}

	locales := map[string]map[string]string{} // lang -> id -> name
	for _, lang := range endfieldLangs {
		m, err := fetchEndfieldLocale(lang)
		if err != nil {
			return nil, err
		}
		locales[lang] = m
	}

	// Oldest-first pass, merging entries that share a name into the oldest one.
	var oldest []Character
	pos := map[string]int{}
	for _, en := range entries {
		name := strings.TrimSpace(en.Fields["name"])
		if name == "" {
			continue
		}
		id := en.Fields["id"]
		if id == "" {
			id = en.Key
		}
		var aliases []string
		for _, lang := range endfieldLangs {
			aliases = append(aliases, locales[lang][id])
		}
		key := strings.ToLower(name)
		if p, ok := pos[key]; ok {
			oldest[p].Aliases = cleanAliases(oldest[p].Name, append(oldest[p].Aliases, aliases...))
			continue
		}
		pos[key] = len(oldest)
		oldest = append(oldest, Character{Name: name, Aliases: cleanAliases(name, aliases)})
	}
	if len(oldest) == 0 {
		return nil, fmt.Errorf("arknights endfield: no characters found")
	}

	// Reverse to newest-first.
	chars := make([]Character, 0, len(oldest))
	for i := len(oldest) - 1; i >= 0; i-- {
		chars = append(chars, oldest[i])
	}

	if err := SaveCommit(e.Name(), sha); err != nil {
		return nil, err
	}
	log.Printf("[Arknights Endfield] %d characters (commit %s)", len(chars), sha[:7])
	return chars, nil
}

func fetchEndfieldLocale(lang string) (map[string]string, error) {
	body, err := get(fmt.Sprintf(endfieldLocale, lang))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", lang, err)
	}
	defer body.Close()
	var loc struct {
		Characters map[string]string `json:"characters"`
	}
	if err := json.NewDecoder(body).Decode(&loc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", lang, err)
	}
	return loc.Characters, nil
}

// cleanAliases trims, drops empties and duplicates, and drops the main name.
func cleanAliases(name string, in []string) []string {
	out := []string{}
	seen := map[string]bool{strings.ToLower(name): true}
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a != "" && !seen[strings.ToLower(a)] {
			seen[strings.ToLower(a)] = true
			out = append(out, a)
		}
	}
	return out
}

// stripCharactersJS removes the header comment, "export const characters =" and
// the trailing semicolon so only the object literal remains.
func stripCharactersJS(src string) string {
	src = strings.TrimSpace(src)
	src = strings.Replace(src, "// src/lib/data/characters.js", "", 1)
	src = strings.TrimSpace(src)
	src = strings.TrimPrefix(src, "export const characters =")
	src = strings.TrimSpace(src)
	return strings.TrimSuffix(src, ";")
}

// ---- minimal JS object-literal reader -------------------------------------
// characters.js is JavaScript, not JSON (unquoted keys, trailing commas), so
// this reads the top-level entries and their string fields, in file order.

type jsEntry struct {
	Key    string
	Fields map[string]string // string-valued fields directly inside the entry
}

type jsTok struct {
	kind byte // 's' string, 'i' identifier, 'p' punctuation
	text string
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func tokenizeJS(src string) []jsTok {
	var toks []jsTok
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\n' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				i = len(src)
			} else {
				i += end + 4
			}
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			var sb strings.Builder
			for j < len(src) && src[j] != c {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case 'u':
						if j+4 < len(src) {
							if r, err := strconv.ParseUint(src[j+1:j+5], 16, 32); err == nil {
								sb.WriteRune(rune(r))
								j += 4
								break
							}
						}
						sb.WriteByte('u')
					default:
						sb.WriteByte(src[j])
					}
					j++
					continue
				}
				sb.WriteByte(src[j])
				j++
			}
			toks = append(toks, jsTok{'s', sb.String()})
			i = j + 1
		case isIdentByte(c):
			j := i
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			toks = append(toks, jsTok{'i', src[i:j]})
			i = j
		default:
			toks = append(toks, jsTok{'p', string(c)})
			i++
		}
	}
	return toks
}

// parseJSObjects returns the entries of `{ key: { field: "value", ... }, ... }`.
func parseJSObjects(src string) []jsEntry {
	toks := tokenizeJS(src)
	var entries []jsEntry
	var cur *jsEntry
	depth := 0
	for i, t := range toks {
		if t.kind == 'p' {
			switch t.text {
			case "{", "[":
				depth++
				if t.text == "{" && depth == 2 && i >= 2 &&
					toks[i-1].kind == 'p' && toks[i-1].text == ":" &&
					(toks[i-2].kind == 'i' || toks[i-2].kind == 's') {
					cur = &jsEntry{Key: toks[i-2].text, Fields: map[string]string{}}
				}
			case "}", "]":
				if depth == 2 && t.text == "}" && cur != nil {
					entries = append(entries, *cur)
					cur = nil
				}
				depth--
			}
			continue
		}
		// field: "string value" directly inside an entry
		if depth == 2 && cur != nil && (t.kind == 'i' || t.kind == 's') &&
			i+2 < len(toks) && toks[i+1].kind == 'p' && toks[i+1].text == ":" && toks[i+2].kind == 's' {
			cur.Fields[t.text] = toks[i+2].text
		}
	}
	return entries
}
