package scrapers

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func sel(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc.Find("body").Children().First()
}

func TestParseItemDate_PrefersDatetimeAttr(t *testing.T) {
	s := sel(t, `<time datetime="2024-03-07T10:00:00Z">last Thursday</time>`)
	got, ok := parseItemDate(s)
	if !ok {
		t.Fatal("expected datetime attr to parse")
	}
	if got.Year() != 2024 || got.Month() != 3 || got.Day() != 7 {
		t.Fatalf("got %v, want 2024-03-07", got)
	}
}

func TestParseItemDate_TextLayouts(t *testing.T) {
	cases := map[string]struct{ y, m, d int }{
		`<span>2024-03-07</span>`:        {2024, 3, 7},
		`<span>Mar 7, 2024</span>`:       {2024, 3, 7},
		`<span>March 7, 2024</span>`:     {2024, 3, 7},
		`<span>7 March 2024</span>`:      {2024, 3, 7},
		`<span>07/03/2024</span>`:        {2024, 3, 7},
		`<span> January 2, 2006 </span>`: {2006, 1, 2},
	}
	for html, want := range cases {
		s := sel(t, html)
		got, ok := parseItemDate(s)
		if !ok {
			t.Errorf("%s: expected parse", html)
			continue
		}
		if got.Year() != want.y || int(got.Month()) != want.m || got.Day() != want.d {
			t.Errorf("%s: got %v, want %04d-%02d-%02d", html, got, want.y, want.m, want.d)
		}
	}
}

func TestParseItemDate_Unparseable(t *testing.T) {
	s := sel(t, `<span>read more</span>`)
	if _, ok := parseItemDate(s); ok {
		t.Fatal("expected non-date text to fail")
	}
}
