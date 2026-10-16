package httpx

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	maxCursorLen    = 512
)

// PageRequest is the parsed limit and cursor of a list request.
type PageRequest struct {
	Limit  int
	Cursor string // decoded; empty for the first page
}

// Page is the response envelope for list endpoints.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// ParsePage reads limit and cursor from the query string. A missing limit uses
// DefaultPageSize. A limit above MaxPageSize, below 1 or not a number is an
// error, because silently clamping hides client bugs.
func ParsePage(r *http.Request) (PageRequest, error) {
	q := r.URL.Query()
	p := PageRequest{Limit: DefaultPageSize}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxPageSize {
			return PageRequest{}, errors.New("limit must be an integer between 1 and 200")
		}
		p.Limit = n
	}
	if v := q.Get("cursor"); v != "" {
		c, err := DecodeCursor(v)
		if err != nil {
			return PageRequest{}, err
		}
		p.Cursor = c
	}
	return p, nil
}

// EncodeCursor turns a position (such as the last id returned) into an opaque
// token. Clients must treat it as opaque. Handlers must still scope every query
// to the caller's tenant, because a cursor proves nothing about access.
func EncodeCursor(position string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(position))
}

// DecodeCursor reverses EncodeCursor.
func DecodeCursor(token string) (string, error) {
	if len(token) > maxCursorLen {
		return "", errors.New("cursor is invalid")
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) == 0 {
		return "", errors.New("cursor is invalid")
	}
	return string(b), nil
}

// NewPage builds a Page from up to limit+1 fetched items. Fetch one extra row
// to learn whether another page exists. next is called with the last item that
// is returned and gives the position for the next cursor.
func NewPage[T any](fetched []T, limit int, next func(last T) string) Page[T] {
	if fetched == nil {
		fetched = []T{}
	}
	if len(fetched) <= limit {
		return Page[T]{Items: fetched}
	}
	items := fetched[:limit]
	return Page[T]{Items: items, NextCursor: EncodeCursor(next(items[limit-1]))}
}
