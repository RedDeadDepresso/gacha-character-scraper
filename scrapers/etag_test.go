package scrapers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceTag(t *testing.T) {
	etags := map[string]string{"/a.json": `"v1"`, "/b.json": `W/"v1"`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method %s, want HEAD", r.Method)
		}
		switch tag, ok := etags[r.URL.Path]; {
		case r.URL.Path == "/noetag.json":
		case !ok:
			http.NotFound(w, r)
		default:
			w.Header().Set("ETag", tag)
		}
	}))
	defer srv.Close()
	a, b, gone := srv.URL+"/a.json", srv.URL+"/b.json", srv.URL+"/gone.json"

	t1, err := SourceTag(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if t2, _ := SourceTag(a, b); t2 != t1 {
		t.Error("same ETags gave a different tag")
	}
	etags["/b.json"] = `W/"v2"`
	if t2, _ := SourceTag(a, b); t2 == t1 {
		t.Error("changed ETag gave the same tag")
	}
	// A weak prefix alone must not change the tag.
	etags["/b.json"] = `"v1"`
	if t2, _ := SourceTag(a, b); t2 != t1 {
		t.Error("weak prefix changed the tag")
	}
	// A missing file is a valid state, and a file appearing later changes the tag.
	m1, err := SourceTag(a, gone)
	if err != nil {
		t.Fatalf("404 must not be an error: %v", err)
	}
	etags["/gone.json"] = `"new"`
	if m2, _ := SourceTag(a, gone); m2 == m1 {
		t.Error("a file appearing did not change the tag")
	}
	if _, err := SourceTag(srv.URL + "/noetag.json"); err == nil {
		t.Error("a response without ETag must be an error")
	}
}

func TestUpToDate(t *testing.T) {
	chars := []Character{{Name: Names{LangEN: "A"}}}
	if upToDate("Nobody", "", chars) {
		t.Error("an empty tag must never be up to date")
	}
	if upToDate("Nobody", "x", chars) {
		t.Error("an unsaved game must not be up to date")
	}
}
