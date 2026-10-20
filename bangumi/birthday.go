package bangumi

import (
	"regexp"
	"strconv"
	"strings"
)

var daysIn = [13]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31} // February: leap day allowed

var monthNames = map[string]int{
	"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3, "april": 4, "apr": 4,
	"may": 5, "june": 6, "jun": 6, "july": 7, "jul": 7, "august": 8, "aug": 8,
	"september": 9, "sept": 9, "sep": 9, "october": 10, "oct": 10, "november": 11, "nov": 11,
	"december": 12, "dec": 12,
}

var (
	reCJK      = regexp.MustCompile(`(\d{1,2})\s*月\s*(\d{1,2})`)                             // 9月28日, 2000年9月28日
	reYMD      = regexp.MustCompile(`\d{4}\s*[-/.]\s*(\d{1,2})\s*[-/.]\s*(\d{1,2})`)         // 2000-09-28
	reMD       = regexp.MustCompile(`(?:^|\D)(\d{1,2})\s*[-/.]\s*(\d{1,2})(?:\D|$)`)         // 9/28, 28/9
	reMonthDay = regexp.MustCompile(`\b([a-z]+)\.?\s*(\d{1,2})(?:st|nd|rd|th)?\b`)           // September 28
	reDayMonth = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)?\s*(?:of\s+)?([a-z]+)\b\.?`) // 28 September
)

func valid(m, d int) bool { return m >= 1 && m <= 12 && d >= 1 && d <= daysIn[m] }

// ParseBirthday reads the month and day out of the free text of a Bangumi
// `生日` field: "9月28日", "2000年9月28日（天秤座）", "9/28", "28/9" (a first number
// above 12 is the day), "2000-09-28", "September 28" or "28th of September".
// A year is ignored. It returns 0, 0 when there is no valid month and day (a
// month alone, "不明", an impossible date such as 2月30日).
func ParseBirthday(s string) (month, day int) {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(foldWidth(r))
	}
	t := strings.ToLower(b.String())
	num := func(x string) int { n, _ := strconv.Atoi(x); return n }

	for _, re := range []*regexp.Regexp{reCJK, reYMD} {
		if m := re.FindStringSubmatch(t); m != nil {
			if mo, d := num(m[1]), num(m[2]); valid(mo, d) {
				return mo, d
			}
		}
	}
	if m := reMD.FindStringSubmatch(t); m != nil {
		a, c := num(m[1]), num(m[2])
		if a > 12 && c <= 12 {
			a, c = c, a
		}
		if valid(a, c) {
			return a, c
		}
	}
	for _, m := range reMonthDay.FindAllStringSubmatch(t, -1) {
		if mo, ok := monthNames[m[1]]; ok && valid(mo, num(m[2])) {
			return mo, num(m[2])
		}
	}
	for _, m := range reDayMonth.FindAllStringSubmatch(t, -1) {
		if mo, ok := monthNames[m[2]]; ok && valid(mo, num(m[1])) {
			return mo, num(m[1])
		}
	}
	return 0, 0
}
