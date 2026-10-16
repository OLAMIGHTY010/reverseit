package server

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

const readyTimeout = 2 * time.Second

// Checker reports whether one dependency is usable.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type health struct {
	service  string
	version  string
	checkers []Checker
}

type liveBody struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

// live answers whether the process is running. It touches no dependency.
func (h *health) live(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, liveBody{Status: "ok", Service: h.service, Version: h.version})
}

type readyBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// ready runs every checker concurrently. It reports only "ok" or "failed" per
// dependency, because error text can contain hostnames or credentials.
func (h *health) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	results := make(map[string]string, len(h.checkers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range h.checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			state := "ok"
			if err := c.Check(ctx); err != nil {
				state = "failed"
			}
			mu.Lock()
			results[c.Name()] = state
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	status, code := "ok", http.StatusOK
	names := make([]string, 0, len(results))
	for n := range results {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if results[n] != "ok" {
			status, code = "unavailable", http.StatusServiceUnavailable
		}
	}
	httpx.WriteJSON(w, code, readyBody{Status: status, Checks: results})
}
