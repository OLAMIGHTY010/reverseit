package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func testTimeouts() Timeouts {
	return Timeouts{Read: time.Second, Write: time.Second, Idle: time.Second, Shutdown: 2 * time.Second}
}

func TestServeStopsOnContextCancel(t *testing.T) {
	logger, _ := quietLogger()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, NewRouter(Options{Logger: logger}), logger, testTimeouts())
	}()

	resp, err := http.Get("http://" + ln.Addr().String() + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}

	if _, err := http.Get("http://" + ln.Addr().String() + "/v1/health"); err == nil {
		t.Fatal("server still accepting connections after shutdown")
	}
}

func TestServeWaitsForInFlightRequest(t *testing.T) {
	logger, _ := quietLogger()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, slow, logger, testTimeouts()) }()

	respc := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			respc <- -1
			return
		}
		resp.Body.Close()
		respc <- resp.StatusCode
	}()

	<-started
	cancel()

	if code := <-respc; code != http.StatusNoContent {
		t.Fatalf("in-flight request got %d, want 204", code)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve returned %v", err)
	}
}
