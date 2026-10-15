package scrapers

import "strings"

// Language codes of the KKAFIO index (the codes of KKAFIO's assets/i18n/).
const (
	LangEN = "en_us"
	LangZH = "zh_cn"
	LangTW = "zh_tw"
	LangJA = "ja_jp"
	LangKO = "ko_kr"
	LangRU = "ru_ru"
)

// langCodes maps the language codes the various data sources use to KKAFIO
// codes. Keys are lowercase with "-" and "_" removed.
var langCodes = map[string]string{
	"en": LangEN, "enus": LangEN, "eng": LangEN,
	"zh": LangZH, "zhcn": LangZH, "zhhans": LangZH, "chs": LangZH, "cn": LangZH,
	"zhtw": LangTW, "zhhant": LangTW, "cht": LangTW, "tw": LangTW,
	"ja": LangJA, "jp": LangJA, "jpn": LangJA, "jajp": LangJA,
	"ko": LangKO, "kr": LangKO, "kor": LangKO, "kokr": LangKO,
	"ru": LangRU, "ruru": LangRU,
}

// LangCode converts a source language code ("zh-Hant", "jp", "kr", ...) to a
// KKAFIO one. It returns "" for languages the index does not use.
func LangCode(src string) string {
	k := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(src))
	return langCodes[k]
}

// Names holds one name per language, keyed by KKAFIO language code. A language
// that is missing falls back to LangEN in KKAFIO.
type Names map[string]string

// EN returns the English name, or the first available one (in a fixed order)
// when there is no English name.
func (n Names) EN() string {
	if s := n[LangEN]; s != "" {
		return s
	}
	for _, l := range []string{LangJA, LangZH, LangTW, LangKO, LangRU} {
		if s := n[l]; s != "" {
			return s
		}
	}
	return ""
}

// clean collapses whitespace and drops empty names and names that are the same
// as the English one (KKAFIO falls back to it anyway). When there is no
// English name the primary one is promoted to LangEN so the entry always has a
// usable key.
func (n Names) clean() Names {
	out := Names{}
	for l, s := range n {
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			out[l] = s
		}
	}
	if out[LangEN] == "" {
		if p := out.EN(); p != "" {
			out[LangEN] = p
		}
	}
	for l, s := range out {
		if l != LangEN && s == out[LangEN] {
			delete(out, l)
		}
	}
	return out
}

// nameBuilder collects the names and aliases of one character. The first name
// added for a language is the name; later different ones become aliases.
type nameBuilder struct {
	names   Names
	aliases []string
}

func newNameBuilder() *nameBuilder { return &nameBuilder{names: Names{}} }

// Name sets the name for a language given as a KKAFIO code or a source code.
// Unknown languages are ignored.
func (b *nameBuilder) Name(lang, text string) *nameBuilder {
	if c := LangCode(lang); c != "" {
		lang = c
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || !isLang(lang) {
		return b
	}
	if cur, ok := b.names[lang]; !ok {
		b.names[lang] = text
	} else if !strings.EqualFold(cur, text) {
		b.aliases = append(b.aliases, text)
	}
	return b
}

// Alias adds names that are not the main name of any language (short names,
// variants, other word orders, ...).
func (b *nameBuilder) Alias(text ...string) *nameBuilder {
	b.aliases = append(b.aliases, text...)
	return b
}

// Merge folds another character into the builder: missing languages are
// filled in, differing names and all aliases become aliases.
func (b *nameBuilder) Merge(c Character) *nameBuilder {
	for l, s := range c.Name {
		b.Name(l, s)
	}
	return b.Alias(c.Aliases...)
}

// Character returns the finished character.
func (b *nameBuilder) Character() Character {
	names := b.names.clean()
	return Character{Name: names, Aliases: cleanAliases(names, b.aliases)}
}

func isLang(l string) bool {
	switch l {
	case LangEN, LangZH, LangTW, LangJA, LangKO, LangRU:
		return true
	}
	return false
}

// builderOf returns a builder seeded with an existing character.
func builderOf(c Character) *nameBuilder {
	b := newNameBuilder()
	for l, s := range c.Name {
		b.names[l] = s
	}
	b.aliases = append(b.aliases, c.Aliases...)
	return b
}

// cleanAliases trims, drops empties and duplicates, and drops anything that is
// already one of the names (every name is searchable, so an alias equal to one
// adds nothing).
func cleanAliases(names Names, in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range names {
		seen[strings.ToLower(s)] = true
	}
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a != "" && !seen[strings.ToLower(a)] {
			seen[strings.ToLower(a)] = true
			out = append(out, a)
		}
	}
	return out
}
