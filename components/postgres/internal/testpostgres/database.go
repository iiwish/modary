// Package testpostgres provides isolated PostgreSQL schemas for repository
// integration tests. It is unavailable to external consumers.
package testpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const defaultURL = "postgres://modary:modary-test-password@127.0.0.1:55432/modary_test?sslmode=disable"

var sequence atomic.Uint64

// Config identifies one isolated application and River schema pair.
type Config struct {
	URL               string
	ApplicationSchema string
	QueueSchema       string
}

// New reserves unique schema names and registers cleanup. The schemas remain
// absent until the PostgreSQL adapter starts, preserving composition purity.
func New(t testing.TB) Config {
	t.Helper()
	url := os.Getenv("MODARY_TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("MODARY_DATABASE_URL")
	}
	if url == "" {
		url = defaultURL
	}
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.PingContext(context.Background()); err != nil {
		_ = admin.Close()
		if strings.Contains(err.Error(), "connection refused") {
			t.Skipf("PostgreSQL integration service unavailable: %v", err)
		}
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d_%d", time.Now().UnixNano(), sequence.Add(1))
	config := Config{
		URL: url, ApplicationSchema: "test_app_" + suffix, QueueSchema: "test_queue_" + suffix,
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + QuoteIdentifier(config.QueueSchema) + ` CASCADE`)
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + QuoteIdentifier(config.ApplicationSchema) + ` CASCADE`)
		_ = admin.Close()
	})
	return config
}

// Open returns a pool whose search_path is pinned to Config.ApplicationSchema.
func Open(t testing.TB, config Config) *sql.DB {
	t.Helper()
	parsed, err := pgx.ParseConfig(config.URL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.RuntimeParams["search_path"] = QuoteIdentifier(config.ApplicationSchema)
	db := stdlib.OpenDB(*parsed)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Minute)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// QuoteIdentifier quotes a trusted PostgreSQL identifier.
func QuoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
