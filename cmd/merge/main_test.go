package main

import (
	"reflect"
	"testing"

	"github.com/reddeaddepresso/gacha-character-scraper/scrapers"
)

func TestMergeAltersSeparators(t *testing.T) {
	nemesis := scraped{Name: scrapers.Names{
		scrapers.LangEN: "Nemesis", scrapers.LangJA: "ネメシス", scrapers.LangKO: "네메시스",
		scrapers.LangZH: "纳美西丝", scrapers.LangTW: "納美西絲",
	}}
	for _, alter := range []string{"Nemesis: Gnosis", "Nemesis : Gnosis", "Nemesis・Gnosis", "Nemesis·Gnosis", "Nemesis • Gnosis"} {
		list := []scraped{
			{Name: scrapers.Names{
				scrapers.LangEN: alter, scrapers.LangJA: "ネメシス・ グノーシス", scrapers.LangKO: "네메시스·연광",
				scrapers.LangZH: "纳美西丝·衍光", scrapers.LangTW: "納美西絲·衍光",
			}},
			nemesis,
			{Name: scrapers.Names{scrapers.LangEN: "Krolik"}},
		}
		got := mergeAlters(list)
		if len(got) != 2 || got[0].Name.EN() != "Nemesis" || got[1].Name.EN() != "Krolik" {
			t.Errorf("%q: unexpected result %+v", alter, got)
			continue
		}
		want := []string{alter, "ネメシス・ グノーシス", "네메시스·연광", "纳美西丝·衍光", "納美西絲·衍光"}
		// Aliases hold every name of the alter (sorted by value after the alias list).
		for _, w := range want {
			if !contains(got[0].Aliases, w) {
				t.Errorf("%q: alias %q missing from %v", alter, w, got[0].Aliases)
			}
		}
		if got[0].Name[scrapers.LangJA] != "ネメシス" {
			t.Errorf("%q: base name changed: %v", alter, got[0].Name)
		}
	}
}

func TestMergeAltersNoBase(t *testing.T) {
	// "Re" is not a character here, so these must stay separate.
	list := []scraped{
		{Name: scrapers.Names{scrapers.LangEN: "Re:Zero"}},
		{Name: scrapers.Names{scrapers.LangEN: "Rem"}},
	}
	if got := mergeAlters(list); !reflect.DeepEqual(got, []scraped{
		{Name: scrapers.Names{scrapers.LangEN: "Re:Zero"}, Aliases: []string{}},
		{Name: scrapers.Names{scrapers.LangEN: "Rem"}, Aliases: []string{}},
	}) {
		t.Errorf("unexpected merge: %+v", got)
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
