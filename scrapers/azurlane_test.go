package scrapers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSObjectToJSON(t *testing.T) {
	src := "/* x */ const ship_data={1e5:{a:'it\\'s \"q\"',b:.65,c:[1,2,],\n// c\nd:\"\\x41\"},2:{a:null,a:true,},};\nconst other={x:1};"
	b, err := jsObjectToJSON(src, "ship_data")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	if got["1e5"]["a"] != `it's "q"` || got["1e5"]["b"] != 0.65 || got["1e5"]["d"] != "A" || got["2"]["a"] != true {
		t.Errorf("unexpected: %s", b)
	}
	if _, err := jsObjectToJSON("const x={a:1", "x"); err == nil {
		t.Error("expected error for unclosed literal")
	}
}

func TestBuildAzurCharacters(t *testing.T) {
	src := `const ship_data={1e5:{en_name:"Universal Bulin",jp_name:"汎用型ブリ",cn_name:"泛用型布里",tw_name:"泛用型布里",date:"2017年05月25日"},` +
		`101039:{en_name:"Cassin (Retrofit)",cn_name:"卡辛.改",date:"2017年11月23日"},` +
		`101030:{en_name:"Cassin",jp_name:"カッシン",cn_name:"卡辛",tw_name:"卡辛",date:"2017年05月25日"},` +
		`301010:{en_name:"Fubuki",jp_name:"吹雪",cn_name:"吹雪",date:"2017年12月07日"},` +
		`11000050:{en_name:"Fubuki",jp_name:"雪不帰",cn_name:"雪不归",date:"2023年11月23日"},` +
		`10300010:{en_name:"Kuon*",jp_name:"クオン",cn_name:"久远",date:"2025年08月"},` +
		`102190:{en_name:"Lena",cn_name:"小海伦娜",tw_name:"小海倫娜",date:"2019年05月30日"}};`
	ships, err := parseAzurShips(src)
	if err != nil {
		t.Fatal(err)
	}
	got := buildAzurCharacters(ships)
	var order []string
	for _, c := range got {
		order = append(order, c.Name[LangEN])
	}
	// Newest first by date; ties (same day) by id; Retrofit skipped; Fubuki merged
	// and placed at its oldest entry.
	if want := []string{"Kuon", "Lena", "Fubuki", "Cassin", "Universal Bulin"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if want := (Names{LangEN: "Lena", LangZH: "小海伦娜", LangTW: "小海倫娜"}); !reflect.DeepEqual(got[1].Name, want) {
		t.Errorf("got %+v, want %+v", got[1].Name, want)
	}
	if want := []string{"雪不归", "雪不帰"}; !reflect.DeepEqual(got[2].Aliases, want) {
		t.Errorf("fubuki aliases = %v, want %v", got[2].Aliases, want)
	}
	// Traditional Chinese equal to Simplified is dropped.
	if _, ok := got[4].Name[LangTW]; ok {
		t.Errorf("tw kept: %+v", got[4].Name)
	}
}

func TestAzurDate(t *testing.T) {
	for in, want := range map[string]int{"2026年07月16日": 20260716, "2017年06月": 20170600, "": 0} {
		if got := azurDate(in); got != want {
			t.Errorf("azurDate(%q) = %d, want %d", in, got, want)
		}
	}
}
