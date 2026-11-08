package scrapers

import (
	"strings"
	"testing"
)

func TestReadNameCSV(t *testing.T) {
	// npc-name-en.csv starts with a byte order mark and has extra columns.
	en := "\ufeffname,trans,scenario,noun\nLyria,露莉亚,,\nVyrn,碧,,\n<span class='x'>Frau,芙劳,,\nEmpty,,,\n"
	m, err := readNameCSV(strings.NewReader(en))
	if err != nil {
		t.Fatal(err)
	}
	if m["Lyria"] != "露莉亚" || m["Frau"] != "芙劳" || len(m) != 3 {
		t.Errorf("got %v", m)
	}
	if _, err := readNameCSV(strings.NewReader("a,b\n1,2\n")); err == nil {
		t.Error("expected an error for a csv without name/trans")
	}
}

func TestSplitVariant(t *testing.T) {
	for in, want := range map[string][2]string{
		"Jeanne d'Arc (Summer)": {"Jeanne d'Arc", "Summer"},
		"Lyria":                 {"Lyria", ""},
		"Water Summer (x)":      {"Water Summer", "x"},
	} {
		if b, v := splitVariant(in); b != want[0] || v != want[1] {
			t.Errorf("splitVariant(%q) = %q, %q", in, b, v)
		}
	}
}

func TestGBFChinese(t *testing.T) {
	n := newGBFNames(
		map[string]string{"ジャンヌ・ダルク": "贞德", "ココ": "可可"},
		map[string]string{"Lyria": "露莉亚"},
	)
	for _, c := range []struct{ en, ja, want string }{
		{"Jeanne d'Arc", "ジャンヌダルク", "贞德"},              // normalised Japanese match
		{"Jeanne d'Arc (Summer)", "ジャンヌダルク", "贞德（泳装）"}, // variant translated
		{"Coco (SSR)", "ココ", "可可"},                     // rarity tag dropped
		{"Lyria (Grand)", "", "露莉亚（Grand）"},            // unknown variant kept; English lookup
		{"Unknown", "ウンノウン", ""},
	} {
		if got := n.Chinese(c.en, c.ja); got != c.want {
			t.Errorf("Chinese(%q, %q) = %q, want %q", c.en, c.ja, got, c.want)
		}
	}
}
