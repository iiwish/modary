package governedpostgres

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const (
	defaultApplicationSchema  = "modary"
	defaultQueueSchema        = "modary_queue"
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = 5
	defaultConnectionLifetime = time.Hour
	// River prefixes its longest notification topic to the schema name. The
	// combined PostgreSQL channel identifier must remain within 63 bytes.
	maxRiverQueueSchemaBytes = 46
)

var schemaPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Options explicitly configures the PostgreSQL durable profile. URL is
// required and may be a PostgreSQL URL or pgx keyword/value connection string.
// The two schemas are created if absent and must be distinct. No option is read
// from process environment or global configuration.
type Options struct {
	URL                   string
	ApplicationSchema     string
	QueueSchema           string
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
	if options.ApplicationSchema == "" {
		options.ApplicationSchema = defaultApplicationSchema
	}
	if options.QueueSchema == "" {
		options.QueueSchema = defaultQueueSchema
	}
	if err := validateSchema("application", options.ApplicationSchema); err != nil {
		return Options{}, nil, err
	}
	if err := validateSchema("queue", options.QueueSchema); err != nil {
		return Options{}, nil, err
	}
	if len(options.QueueSchema) > maxRiverQueueSchemaBytes {
		return Options{}, nil, fmt.Errorf("PostgreSQL queue schema must be at most %d bytes for River notifications", maxRiverQueueSchemaBytes)
	}
	if options.ApplicationSchema == options.QueueSchema {
		return Options{}, nil, fmt.Errorf("PostgreSQL application and queue schemas must be distinct")
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
	config.RuntimeParams["search_path"] = quoteIdentifier(options.ApplicationSchema)
	return options, config, nil
}

func validateSchema(role, schema string) error {
	if !utf8.ValidString(schema) || !schemaPattern.MatchString(schema) {
		return fmt.Errorf("PostgreSQL %s schema %q must match %s", role, schema, schemaPattern)
	}
	if schema == "public" || schema == "information_schema" || strings.HasPrefix(schema, "pg_") {
		return fmt.Errorf("PostgreSQL %s schema %q is reserved", role, schema)
	}
	return nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
