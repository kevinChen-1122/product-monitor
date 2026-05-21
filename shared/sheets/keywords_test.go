package sheets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseKeywordsCSV_byIndex(t *testing.T) {
	csv := "keyword,note\niphone,手機\nps5,遊戲\n"
	kws, err := parseKeywordsCSV(strings.NewReader(csv), "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(kws) != 2 || kws[0] != "iphone" || kws[1] != "ps5" {
		t.Fatalf("got %v", kws)
	}
}

func TestParseKeywordsCSV_byHeader(t *testing.T) {
	csv := "關鍵字,備註\nmac,筆電\n"
	kws, err := parseKeywordsCSV(strings.NewReader(csv), "關鍵字")
	if err != nil {
		t.Fatal(err)
	}
	if len(kws) != 1 || kws[0] != "mac" {
		t.Fatalf("got %v", kws)
	}
}

func TestParseKeywordsCSV_dedupe(t *testing.T) {
	csv := "kw\na\na\nb\n"
	kws, err := parseKeywordsCSV(strings.NewReader(csv), "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(kws) != 2 {
		t.Fatalf("got %v", kws)
	}
}

func TestFetchKeywords_http(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("keyword\nswitch\n"))
	}))
	defer srv.Close()

	kws, err := FetchKeywords(context.Background(), srv.URL, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(kws) != 1 || kws[0] != "switch" {
		t.Fatalf("got %v", kws)
	}
}

func TestBuildExportURL(t *testing.T) {
	got := BuildExportURL("abc123", "456")
	want := "https://docs.google.com/spreadsheets/d/abc123/export?format=csv&gid=456"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeSheetURL(t *testing.T) {
	raw := "https://docs.google.com/spreadsheets/d/abc123/edit#gid=99"
	got := NormalizeSheetURL(raw)
	if !strings.Contains(got, "abc123/export?format=csv") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "gid=99") {
		t.Fatalf("missing gid: %q", got)
	}
}
