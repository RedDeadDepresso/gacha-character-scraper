package scrapers

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var jsNumberRe = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][+-]?\d+)?$`)

// jsObjectToJSON converts the first JavaScript object literal that follows
// marker (or the first one in src when marker is empty or absent) into JSON.
// It understands what minified data files use: unquoted keys, single or
// double quoted strings, numbers such as 1e5 and .65, trailing commas and
// comments. Everything before the literal (e.g. "const ship_data=") and after
// its closing brace (e.g. the ";") is ignored.
func jsObjectToJSON(src, marker string) ([]byte, error) {
	if i := strings.Index(src, marker); marker != "" && i >= 0 {
		src = src[i+len(marker):]
	}
	start := strings.IndexAny(src, "{[")
	if start < 0 {
		return nil, fmt.Errorf("no object literal found")
	}
	src = src[start:]

	var out strings.Builder
	out.Grow(len(src))
	depth := 0
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			i += end + 4
		case c == '"' || c == '\'':
			j, err := writeJSString(&out, src, i)
			if err != nil {
				return nil, err
			}
			i = j
		case c == '{' || c == '[':
			depth++
			out.WriteByte(c)
			i++
		case c == '}' || c == ']':
			depth--
			out.WriteByte(c)
			i++
			if depth == 0 {
				return []byte(out.String()), nil
			}
		case c == ',':
			// Drop trailing commas.
			j := i + 1
			for j < len(src) && strings.IndexByte(" \t\r\n", src[j]) >= 0 {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				i++
				continue
			}
			out.WriteByte(c)
			i++
		case c == ':':
			out.WriteByte(c)
			i++
		default:
			j := i
			for j < len(src) && isJSWordByte(src[j]) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("unexpected %q at offset %d", c, start+i)
			}
			word := src[i:j]
			k := j
			for k < len(src) && strings.IndexByte(" \t\r\n", src[k]) >= 0 {
				k++
			}
			if k < len(src) && src[k] == ':' { // object key
				fmt.Fprintf(&out, "%q", word)
			} else {
				out.WriteString(jsValueWord(word))
			}
			i = j
		}
	}
	return nil, fmt.Errorf("object literal is not closed")
}

func isJSWordByte(c byte) bool {
	return c == '_' || c == '$' || c == '.' || c == '+' || c == '-' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// jsValueWord converts a bare word used as a value to JSON.
func jsValueWord(w string) string {
	switch w {
	case "true", "false", "null":
		return w
	case "undefined", "NaN", "Infinity", "-Infinity":
		return "null"
	}
	n := w
	switch {
	case strings.HasPrefix(n, "."):
		n = "0" + n
	case strings.HasPrefix(n, "-."):
		n = "-0" + n[1:]
	}
	n = strings.TrimPrefix(n, "+")
	if jsNumberRe.MatchString(n) {
		return n
	}
	return fmt.Sprintf("%q", w) // unknown word: keep it as a string
}

// writeJSString copies the JS string literal starting at src[i] (a quote) to
// out as a JSON string and returns the index after its closing quote.
func writeJSString(out *strings.Builder, src string, i int) (int, error) {
	quote := src[i]
	out.WriteByte('"')
	for j := i + 1; j < len(src); {
		c := src[j]
		switch {
		case c == quote:
			out.WriteByte('"')
			return j + 1, nil
		case c == '"': // only inside a single-quoted string
			out.WriteString(`\"`)
			j++
		case c == '\\':
			if j+1 >= len(src) {
				return 0, fmt.Errorf("unterminated string")
			}
			e := src[j+1]
			j += 2
			switch e {
			case '\'':
				out.WriteByte('\'')
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
				out.WriteByte('\\')
				out.WriteByte(e)
			case 'v':
				out.WriteString(`\u000b`)
			case '0':
				out.WriteString(`\u0000`)
			case 'x':
				if j+2 > len(src) {
					return 0, fmt.Errorf("bad \\x escape")
				}
				fmt.Fprintf(out, `\u00%s`, src[j:j+2])
				j += 2
			case '\n': // line continuation
			case '\r':
				if j < len(src) && src[j] == '\n' {
					j++
				}
			default:
				out.WriteByte(e)
			}
		case c < 0x20:
			fmt.Fprintf(out, `\u%04x`, c)
			j++
		default:
			_, size := utf8.DecodeRuneInString(src[j:])
			out.WriteString(src[j : j+size])
			j += size
		}
	}
	return 0, fmt.Errorf("unterminated string")
}
