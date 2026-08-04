package postgresdb

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	defaultSchema             = "modary"
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = 5
	defaultConnectionLifetime = time.Hour
)

var schemaPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Options explicitly configures the general PostgreSQL component. No value is
// read from global configuration or process environment.
type Options struct {
	URL                   string
	Schema                string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
}

func normalizeOptions(options Options) (Options, *pgx.ConnConfig, error) {
	if !utf8.ValidString(options.URL) || strings.TrimSpace(options.URL) != options.URL || options.URL == "" {
		return Options{}, nil, fmt.Errorf("PostgreSQL URL must be a non-empty valid UTF-8 value without surrounding whitespace")
	}
	config, err := pgx.ParseConfig(options.URL)
	if err != nil {
		return Options{}, nil, fmt.Errorf("PostgreSQL URL is invalid")
	}
	if config.Database == "" {
		return Options{}, nil, fmt.Errorf("PostgreSQL URL must select a database")
	}
	if options.Schema == "" {
		options.Schema = defaultSchema
	}
	if !schemaPattern.MatchString(options.Schema) || options.Schema == "public" || options.Schema == "information_schema" || strings.HasPrefix(options.Schema, "pg_") {
		return Options{}, nil, fmt.Errorf("PostgreSQL schema %q must be a non-reserved identifier matching %s", options.Schema, schemaPattern)
	}
	if options.MaxOpenConnections < 0 || options.MaxIdleConnections < 0 {
		return Options{}, nil, fmt.Errorf("PostgreSQL connection limits cannot be negative")
	}
	if options.MaxOpenConnections == 0 {
		options.MaxOpenConnections = defaultMaxOpenConnections
	}
	if options.MaxIdleConnections == 0 {
		options.MaxIdleConnections = defaultMaxIdleConnections
	}
	if options.MaxIdleConnections > options.MaxOpenConnections {
		return Options{}, nil, fmt.Errorf("PostgreSQL maximum idle connections cannot exceed maximum open connections")
	}
	if options.ConnectionMaxLifetime < 0 {
		return Options{}, nil, fmt.Errorf("PostgreSQL connection lifetime cannot be negative")
	}
	if options.ConnectionMaxLifetime == 0 {
		options.ConnectionMaxLifetime = defaultConnectionLifetime
	}
	config.RuntimeParams["search_path"] = quoteIdentifier(options.Schema)
	return options, config, nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
