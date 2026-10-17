package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environments the service recognises.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// placeholderSecrets are values shipped in .env.example. They are acceptable
// for local development only.
var placeholderSecrets = []string{
	"development_secret_only",
}

// Config holds runtime configuration for the SMARTREVERSAL backend processes.
type Config struct {
	Environment string
	LogLevel    string

	APIPort         int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	DBMaxConns        int
	DBConnMaxLifetime time.Duration

	RedisURL      string
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int

	// JWTKeyID names the signing key. JWTPrivateKey is the base64 Ed25519 seed
	// or private key of the issuing process, and JWTPublicKeys lists extra
	// verification keys as "kid:base64,kid:base64". All are optional here and
	// checked by the identity package in the processes that need them, so the
	// migrate and worker commands do not need a signing key.
	JWTKeyID      string
	JWTPrivateKey string
	JWTPublicKeys string
	JWTTokenTTL   time.Duration

	// AccountTokenKey and AccountEncKey are base64 32-byte keys for account
	// tokens and the sealed account vault (ADR 0003, D10). Optional here; the
	// API leaves the unmask route out when they are missing.
	AccountTokenKey string
	AccountEncKey   string
	AccountEncKeyID string

	AIServiceURL string

	WorkerHealthPort   int
	OutboxPollInterval time.Duration
	OutboxBatchSize    int

	// FinancialExecutionEnabled gates every call to a reversal provider. It
	// defaults to false and must be set explicitly.
	FinancialExecutionEnabled bool
	// CustomerDevLoginEnabled turns on the development sign-in for synthetic ReverseIt
	// customers. It is refused in production, where a real identity provider is
	// required.
	CustomerDevLoginEnabled bool
	// OwnBankNames are names that identify this bank in an alert, comma separated in
	// OWN_BANK_NAMES. They let the alert parser tell another bank's alert from ours.
	OwnBankNames []string
}

// Load reads configuration from the process environment.
func Load() (*Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads configuration through lookup, which lets tests supply values
// without touching the process environment. All problems are reported together.
func LoadFrom(lookup func(string) (string, bool)) (*Config, error) {
	l := &loader{lookup: lookup}

	c := &Config{
		Environment:       strings.ToLower(l.str("ENVIRONMENT", EnvDevelopment)),
		LogLevel:          strings.ToLower(l.str("LOG_LEVEL", "info")),
		APIPort:           l.port("API_PORT", 8080),
		ReadTimeout:       l.duration("API_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      l.duration("API_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       l.duration("API_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   l.duration("API_SHUTDOWN_TIMEOUT", 20*time.Second),
		DBHost:            l.str("DB_HOST", "localhost"),
		DBPort:            l.port("DB_PORT", 5432),
		DBUser:            l.required("DB_USER"),
		DBPassword:        l.required("DB_PASSWORD"),
		DBName:            l.required("DB_NAME"),
		DBSSLMode:         l.str("DB_SSLMODE", "disable"),
		DBMaxConns:        l.positiveInt("DB_MAX_OPEN_CONNS", 25),
		DBConnMaxLifetime: l.duration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		RedisURL:          l.str("REDIS_URL", ""),
		RedisHost:         l.str("REDIS_HOST", "localhost"),
		RedisPort:         l.port("REDIS_PORT", 6379),
		RedisPassword:     l.str("REDIS_PASSWORD", ""),
		RedisDB:           l.nonNegInt("REDIS_DB", 0),
		JWTKeyID:          l.str("JWT_KEY_ID", "k1"),
		JWTPrivateKey:     l.str("JWT_PRIVATE_KEY", ""),
		JWTPublicKeys:     l.str("JWT_PUBLIC_KEYS", ""),
		JWTTokenTTL:       l.duration("JWT_TOKEN_TTL", 15*time.Minute),

		AccountTokenKey: l.str("ACCOUNT_TOKEN_KEY", ""),
		AccountEncKey:   l.str("ACCOUNT_ENC_KEY", ""),
		AccountEncKeyID: l.str("ACCOUNT_ENC_KEY_ID", "k1"),
		AIServiceURL:    l.str("AI_SERVICE_URL", "http://localhost:8000"),

		WorkerHealthPort:   l.port("WORKER_HEALTH_PORT", 8081),
		OutboxPollInterval: l.duration("OUTBOX_POLL_INTERVAL", time.Second),
		OutboxBatchSize:    l.positiveInt("OUTBOX_BATCH_SIZE", 100),

		FinancialExecutionEnabled: l.boolean("FINANCIAL_EXECUTION_ENABLED", false),
		CustomerDevLoginEnabled:   l.boolean("CUSTOMER_DEV_LOGIN_ENABLED", false),
		OwnBankNames:              splitList(l.str("OWN_BANK_NAMES", "")),
	}

	c.validate(l)

	if len(l.errs) > 0 {
		return nil, errors.Join(l.errs...)
	}
	return c, nil
}

func (c *Config) validate(l *loader) {
	switch c.Environment {
	case EnvDevelopment, EnvTest, EnvStaging, EnvProduction:
	default:
		l.errs = append(l.errs, fmt.Errorf("ENVIRONMENT %q is not one of development, test, staging, production", c.Environment))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		l.errs = append(l.errs, fmt.Errorf("LOG_LEVEL %q is not one of debug, info, warn, error", c.LogLevel))
	}

	if c.Environment == EnvStaging || c.Environment == EnvProduction {
		for _, p := range placeholderSecrets {
			if c.DBPassword == p {
				l.errs = append(l.errs, fmt.Errorf("the placeholder database password from .env.example is not allowed in %s", c.Environment))
				break
			}
		}
		if c.DBSSLMode == "disable" {
			l.errs = append(l.errs, fmt.Errorf("DB_SSLMODE=disable is not allowed in %s", c.Environment))
		}
	}

	if c.CustomerDevLoginEnabled && c.Environment == EnvProduction {
		l.errs = append(l.errs, errors.New("CUSTOMER_DEV_LOGIN_ENABLED cannot be true in production: customers must sign in through the bank's identity provider"))
	}
	if c.FinancialExecutionEnabled && c.Environment == EnvProduction {
		l.errs = append(l.errs, errors.New("FINANCIAL_EXECUTION_ENABLED cannot be true in production until the required approvals are recorded and this check is changed"))
	}
}

// DatabaseURL returns a PostgreSQL connection URL with credentials escaped.
func (c *Config) DatabaseURL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     fmt.Sprintf("%s:%d", c.DBHost, c.DBPort),
		Path:     c.DBName,
		RawQuery: url.Values{"sslmode": {c.DBSSLMode}}.Encode(),
	}
	return u.String()
}

// RedisAddr returns host:port for the Redis client.
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.RedisHost, c.RedisPort)
}

// LogValue implements slog.LogValuer so that logging a Config never prints
// secrets.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("environment", c.Environment),
		slog.Int("api_port", c.APIPort),
		slog.String("db_host", c.DBHost),
		slog.Int("db_port", c.DBPort),
		slog.String("db_name", c.DBName),
		slog.String("redis_addr", c.RedisAddr()),
		slog.Bool("financial_execution_enabled", c.FinancialExecutionEnabled),
		slog.Bool("customer_dev_login_enabled", c.CustomerDevLoginEnabled),
	)
}

type loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (l *loader) raw(key string) (string, bool) {
	v, ok := l.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (l *loader) str(key, def string) string {
	if v, ok := l.raw(key); ok {
		return v
	}
	return def
}

func (l *loader) required(key string) string {
	v, ok := l.raw(key)
	if !ok {
		l.errs = append(l.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (l *loader) port(key string, def int) int {
	v, ok := l.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a port between 1 and 65535", key))
		return def
	}
	return n
}

func (l *loader) nonNegInt(key string, def int) int {
	v, ok := l.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a non-negative integer", key))
		return def
	}
	return n
}

func (l *loader) positiveInt(key string, def int) int {
	v, ok := l.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a positive integer", key))
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v, ok := l.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a positive duration such as 15s", key))
		return def
	}
	return d
}

func (l *loader) boolean(key string, def bool) bool {
	v, ok := l.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s must be true or false", key))
		return def
	}
	return b
}

// splitList splits a comma separated value, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
