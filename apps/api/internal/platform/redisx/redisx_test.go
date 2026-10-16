package redisx

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRequiresAddr(t *testing.T) {
	if _, err := Open(context.Background(), Options{}); err == nil {
		t.Fatal("empty address accepted")
	}
}

func TestOpenFailureDoesNotLeakPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Open(ctx, Options{Addr: "127.0.0.1:1", Password: "s3cr3t-redis-pw"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cr3t-redis-pw") {
		t.Fatalf("password leaked: %v", err)
	}
}

func TestCheckAgainstRealRedis(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Open(ctx, Options{Addr: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Name() != "redis" {
		t.Fatalf("name = %s", c.Name())
	}
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
}
