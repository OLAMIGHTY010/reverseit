package db

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration tests need a disposable PostgreSQL. Set TEST_DATABASE_URL to run
// them, for example postgres://app:testpw@localhost:55432/sr_test?sslmode=disable
func testPool(t *testing.T) *Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := Open(ctx, url, Options{MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestOpenRejectsBadURLWithoutLeakingPassword(t *testing.T) {
	_, err := Open(context.Background(), "postgres://u:s3cr3t-pw@127.0.0.1:1/x?sslmode=disable&connect_timeout=1", Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "s3cr3t-pw") {
		t.Fatalf("password leaked: %v", err)
	}

	_, err = Open(context.Background(), "::not a url::s3cr3t-pw", Options{})
	if err == nil || strings.Contains(err.Error(), "s3cr3t-pw") {
		t.Fatalf("bad config error missing or leaked: %v", err)
	}
}

func TestCheckAndName(t *testing.T) {
	p := testPool(t)
	if p.Name() != "postgres" {
		t.Fatalf("name = %s", p.Name())
	}
	if err := p.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWithTxCommitAndRollback(t *testing.T) {
	p := testPool(t)
	ctx := context.Background()

	if _, err := p.Exec(ctx, `DROP TABLE IF EXISTS db_pkg_tx_test; CREATE TABLE db_pkg_tx_test (id int PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = p.Exec(context.Background(), `DROP TABLE IF EXISTS db_pkg_tx_test`) })

	count := func() int {
		var n int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM db_pkg_tx_test`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	err := p.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO db_pkg_tx_test VALUES (1)`)
		return err
	})
	if err != nil || count() != 1 {
		t.Fatalf("commit failed: err=%v count=%d", err, count())
	}

	boom := errors.New("boom")
	err = p.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO db_pkg_tx_test VALUES (2)`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || count() != 1 {
		t.Fatalf("rollback on error failed: err=%v count=%d", err, count())
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		_ = p.WithTx(ctx, func(tx pgx.Tx) error {
			_, _ = tx.Exec(ctx, `INSERT INTO db_pkg_tx_test VALUES (3)`)
			panic("fail mid transaction")
		})
	}()
	if count() != 1 {
		t.Fatalf("rollback on panic failed: count=%d", count())
	}
}
