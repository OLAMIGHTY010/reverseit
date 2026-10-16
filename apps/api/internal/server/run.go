package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Serve runs handler on ln until ctx is cancelled, then stops accepting new
// connections and waits up to shutdownTimeout for in-flight requests.
func Serve(ctx context.Context, ln net.Listener, handler http.Handler, logger *slog.Logger, t Timeouts) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       t.Read,
		WriteTimeout:      t.Write,
		IdleTimeout:       t.Idle,
		MaxHeaderBytes:    1 << 16,
	}

	errc := make(chan error, 1)
	go func() {
		logger.Info("http_listening", slog.String("addr", ln.Addr().String()))
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	logger.Info("http_shutting_down")
	shutCtx, cancel := context.WithTimeout(context.Background(), t.Shutdown)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		_ = srv.Close()
		return err
	}
	return <-errc
}

// Timeouts groups the server timeouts.
type Timeouts struct {
	Read     time.Duration
	Write    time.Duration
	Idle     time.Duration
	Shutdown time.Duration
}
