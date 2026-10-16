// Command mock runs one simulated external service: status, reversal or notify.
// It is a Track H stand-in and never talks to a real system.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/mocks"
	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
	"github.com/smartreversal/smartreversal/apps/api/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mock:", err)
		os.Exit(1)
	}
}

func run() error {
	kind, err := mocks.ParseKind(os.Getenv("MOCK_KIND"))
	if err != nil {
		return fmt.Errorf("MOCK_KIND: %w", err)
	}
	port := 9100
	if v := os.Getenv("MOCK_PORT"); v != "" {
		port, err = strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("MOCK_PORT must be a port between 1 and 65535")
		}
	}

	logger := observability.NewLogger("production", "info")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return server.Serve(ctx, ln, mocks.NewHandler(kind, logger), logger, server.Timeouts{
		Read: 10 * time.Second, Write: 10 * time.Second, Idle: 30 * time.Second, Shutdown: 10 * time.Second,
	})
}
