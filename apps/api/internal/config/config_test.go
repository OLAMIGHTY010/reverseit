package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func base() map[string]string {
	return map[string]string{
		"DB_USER":     "app",
		"DB_PASSWORD": "pw",
		"DB_NAME":     "db",
	}
}

func with(m map[string]string, kv ...string) map[string]string {
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func TestLoadDefaults(t *testing.T) {
	c, err := LoadFrom(env(base()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Environment != EnvDevelopment || c.APIPort != 8080 || c.DBPort != 5432 || c.RedisPort != 6379 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.ReadTimeout != 15*time.Second || c.ShutdownTimeout != 20*time.Second {
		t.Fatalf("unexpected timeouts: %+v", c)
	}
	if c.WorkerHealthPort != 8081 || c.OutboxPollInterval != time.Second || c.OutboxBatchSize != 100 {
		t.Fatalf("unexpected worker defaults: %+v", c)
	}
	if c.DBMaxConns != 25 || c.DBConnMaxLifetime != 5*time.Minute {
		t.Fatalf("unexpected pool defaults: %d %s", c.DBMaxConns, c.DBConnMaxLifetime)
	}
	if c.FinancialExecutionEnabled {
		t.Fatal("financial execution must default to false")
	}
}

func TestLoadReportsAllMissingRequired(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error should mention %s: %v", k, err)
		}
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"bad token ttl":   with(base(), "JWT_TOKEN_TTL", "soon"),
		"bad port":        with(base(), "API_PORT", "70000"),
		"non numeric":     with(base(), "DB_PORT", "abc"),
		"bad duration":    with(base(), "API_READ_TIMEOUT", "fast"),
		"zero duration":   with(base(), "API_IDLE_TIMEOUT", "0s"),
		"bad environment": with(base(), "ENVIRONMENT", "prod"),
		"bad log level":   with(base(), "LOG_LEVEL", "trace"),
		"bad bool":        with(base(), "FINANCIAL_EXECUTION_ENABLED", "maybe"),
		"negative redis":  with(base(), "REDIS_DB", "-1"),
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFrom(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPlaceholderDatabasePasswordOnlyAllowedInDevelopment(t *testing.T) {
	placeholder := placeholderSecrets[0]

	if _, err := LoadFrom(env(with(base(), "DB_PASSWORD", placeholder))); err != nil {
		t.Fatalf("development should accept placeholder: %v", err)
	}
	for _, e := range []string{EnvStaging, EnvProduction} {
		m := with(base(), "DB_PASSWORD", placeholder, "ENVIRONMENT", e, "DB_SSLMODE", "require")
		if _, err := LoadFrom(env(m)); err == nil {
			t.Errorf("%s accepted the placeholder database password", e)
		}
	}
}

func TestSigningKeySettingsAreOptionalAndHaveDefaults(t *testing.T) {
	c, err := LoadFrom(env(base()))
	if err != nil {
		t.Fatalf("migrate and worker must load without signing keys: %v", err)
	}
	if c.JWTKeyID != "k1" || c.JWTTokenTTL != 15*time.Minute || c.JWTPrivateKey != "" || c.JWTPublicKeys != "" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	c, err = LoadFrom(env(with(base(), "JWT_KEY_ID", "2026-10", "JWT_PRIVATE_KEY", "abc", "JWT_PUBLIC_KEYS", "k0:def", "JWT_TOKEN_TTL", "5m")))
	if err != nil || c.JWTKeyID != "2026-10" || c.JWTPrivateKey != "abc" || c.JWTPublicKeys != "k0:def" || c.JWTTokenTTL != 5*time.Minute {
		t.Fatalf("values not read: %+v %v", c, err)
	}
}

func TestAccountKeySettings(t *testing.T) {
	c, err := LoadFrom(env(base()))
	if err != nil || c.AccountTokenKey != "" || c.AccountEncKey != "" || c.AccountEncKeyID != "k1" {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	c, err = LoadFrom(env(with(base(), "ACCOUNT_TOKEN_KEY", "t", "ACCOUNT_ENC_KEY", "e", "ACCOUNT_ENC_KEY_ID", "2026-10")))
	if err != nil || c.AccountTokenKey != "t" || c.AccountEncKey != "e" || c.AccountEncKeyID != "2026-10" {
		t.Fatalf("values: %+v %v", c, err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", c)
	if strings.Contains(buf.String(), "ACCOUNT") || strings.Contains(buf.String(), "=e ") {
		t.Fatalf("account keys leaked into the log: %s", buf.String())
	}
}

func TestLoggingConfigNeverIncludesSigningKey(t *testing.T) {
	c, err := LoadFrom(env(with(base(), "JWT_PRIVATE_KEY", "private-key-material")))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", c)
	if strings.Contains(buf.String(), "private-key-material") {
		t.Fatalf("signing key leaked: %s", buf.String())
	}
}

func TestProductionRequiresTLSToDatabase(t *testing.T) {
	m := with(base(), "ENVIRONMENT", EnvProduction)
	if _, err := LoadFrom(env(m)); err == nil {
		t.Fatal("production accepted sslmode=disable")
	}
	m["DB_SSLMODE"] = "verify-full"
	if _, err := LoadFrom(env(m)); err != nil {
		t.Fatalf("production with TLS rejected: %v", err)
	}
}

func TestFinancialExecutionBlockedInProduction(t *testing.T) {
	m := with(base(), "FINANCIAL_EXECUTION_ENABLED", "true")
	c, err := LoadFrom(env(m))
	if err != nil || !c.FinancialExecutionEnabled {
		t.Fatalf("development should allow explicit enable: %v", err)
	}
	m = with(base(), "FINANCIAL_EXECUTION_ENABLED", "true", "ENVIRONMENT", EnvProduction, "DB_SSLMODE", "require")
	if _, err := LoadFrom(env(m)); err == nil {
		t.Fatal("production accepted financial execution")
	}
}

func TestDatabaseURLEscapesCredentials(t *testing.T) {
	m := with(base(), "DB_PASSWORD", "p@ss/w:rd", "DB_USER", "us er")
	c, err := LoadFrom(env(m))
	if err != nil {
		t.Fatal(err)
	}
	got := c.DatabaseURL()
	want := "postgres://us%20er:p%40ss%2Fw%3Ard@localhost:5432/db?sslmode=disable"
	if got != want {
		t.Fatalf("DatabaseURL = %s, want %s", got, want)
	}
}

func TestLoggingConfigDoesNotLeakSecrets(t *testing.T) {
	c, err := LoadFrom(env(with(base(), "DB_PASSWORD", "very-secret-db-password")))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", c)
	out := buf.String()
	if strings.Contains(out, "very-secret-db-password") {
		t.Fatalf("secret leaked into log: %s", out)
	}
	if !strings.Contains(out, "financial_execution_enabled=false") {
		t.Fatalf("expected safe fields in log: %s", out)
	}
}

func TestTheCustomerDevelopmentLoginIsOffByDefaultAndRefusedInProduction(t *testing.T) {
	c, err := LoadFrom(env(base()))
	if err != nil || c.CustomerDevLoginEnabled {
		t.Fatalf("%v %v", err, c)
	}
	m := with(base(), "CUSTOMER_DEV_LOGIN_ENABLED", "true")
	if c, err := LoadFrom(env(m)); err != nil || !c.CustomerDevLoginEnabled {
		t.Fatalf("it should be allowed in development: %v", err)
	}
	m = with(base(), "CUSTOMER_DEV_LOGIN_ENABLED", "true", "ENVIRONMENT", EnvProduction, "DB_SSLMODE", "require")
	if _, err := LoadFrom(env(m)); err == nil || !strings.Contains(err.Error(), "CUSTOMER_DEV_LOGIN_ENABLED") {
		t.Fatalf("production accepted the development login: %v", err)
	}
}
