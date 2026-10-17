// Package redisx opens the Redis client used for streams. Redis is a delivery
// channel and cache. It never holds the authoritative copy of a financial fact.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options configures the client.
type Options struct {
	URL      string
	Addr     string
	Password string
	DB       int
}

// Client wraps redis.Client. It satisfies the server readiness Checker.
type Client struct {
	*redis.Client
}

// Open creates a client and verifies connectivity. Errors never include the
// password.
func Open(ctx context.Context, o Options) (*Client, error) {
	var opt *redis.Options
	if o.URL != "" {
		parsed, err := redis.ParseURL(o.URL)
		if err != nil {
			return nil, fmt.Errorf("redis: invalid url: %w", err)
		}
		opt = parsed
	} else {
		if o.Addr == "" {
			return nil, errors.New("redis: address is required")
		}
		opt = &redis.Options{
			Addr:     o.Addr,
			Password: o.Password,
			DB:       o.DB,
		}
	}
	opt.DialTimeout = 3 * time.Second
	opt.ReadTimeout = 5 * time.Second
	opt.WriteTimeout = 5 * time.Second

	c := &Client{Client: redis.NewClient(opt)}
	if err := c.Check(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis: initial ping failed: %w", errors.Unwrap(err))
	}
	return c, nil
}

// Name identifies the dependency in readiness output.
func (c *Client) Name() string { return "redis" }

// Check pings Redis.
func (c *Client) Check(ctx context.Context) error {
	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
