package httpx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func req(query string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/x?"+query, nil)
}

func TestParsePageDefaultsAndValues(t *testing.T) {
	p, err := ParsePage(req(""))
	if err != nil || p.Limit != DefaultPageSize || p.Cursor != "" {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p, err = ParsePage(req("limit=200&cursor=" + EncodeCursor("abc123")))
	if err != nil || p.Limit != 200 || p.Cursor != "abc123" {
		t.Fatalf("values: %+v %v", p, err)
	}
}

func TestParsePageRejectsBadInput(t *testing.T) {
	for _, q := range []string{
		"limit=0", "limit=-1", "limit=201", "limit=abc", "limit=1.5",
		"cursor=!!!notbase64", "cursor=" + strings.Repeat("A", maxCursorLen+1),
	} {
		if _, err := ParsePage(req(q)); err == nil {
			t.Errorf("%q accepted", q)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	for _, pos := range []string{"1", "TXN-0001842", "2026-10-14T09:12:41Z|99", "ünïcode"} {
		got, err := DecodeCursor(EncodeCursor(pos))
		if err != nil || got != pos {
			t.Errorf("round trip %q -> %q, %v", pos, got, err)
		}
	}
	if _, err := DecodeCursor(""); err == nil {
		t.Error("empty cursor accepted")
	}
}

func TestNewPage(t *testing.T) {
	next := func(s string) string { return s }

	// Fewer than limit+1: no next page.
	pg := NewPage([]string{"a", "b"}, 2, next)
	if !reflect.DeepEqual(pg.Items, []string{"a", "b"}) || pg.NextCursor != "" {
		t.Fatalf("last page: %+v", pg)
	}

	// limit+1 fetched: trim and point at the last returned item.
	pg = NewPage([]string{"a", "b", "c"}, 2, next)
	if !reflect.DeepEqual(pg.Items, []string{"a", "b"}) {
		t.Fatalf("items: %+v", pg.Items)
	}
	if pos, _ := DecodeCursor(pg.NextCursor); pos != "b" {
		t.Fatalf("cursor points at %q, want b", pos)
	}

	// nil input still produces an empty JSON array.
	pg = NewPage[string](nil, 10, next)
	if pg.Items == nil || len(pg.Items) != 0 {
		t.Fatalf("nil input: %+v", pg)
	}
}
