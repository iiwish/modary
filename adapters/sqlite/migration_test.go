package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iiwish/modary/internal/databasecontrol"
)

func TestMigrationRetryChecksumAndPrefixSafety(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	control, err := databasecontrol.New(&backend{db: db})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const migrationModule = "sqlite-test"
	one := []byte(`CREATE TABLE migration_retry_one (id INTEGER PRIMARY KEY) STRICT;`)
	broken := fstest.MapFS{
		"0001_one.sql": {Data: one},
		"0002_two.sql": {Data: []byte(`CREATE TABLE migration_retry_one (id INTEGER PRIMARY KEY) STRICT;`)},
	}
	if err := control.ApplyMigrations(ctx, migrationModule, broken); err == nil {
		t.Fatal("broken migration batch succeeded")
	}
	assertTableExists(t, db, "migration_retry_one", false)
	assertTableExists(t, db, "modary_module_migration", false)

	fixed := fstest.MapFS{
		"0001_one.sql": {Data: one},
		"0002_two.sql": {Data: []byte(`CREATE TABLE migration_retry_two (id INTEGER PRIMARY KEY) STRICT;`)},
	}
	if err := control.ApplyMigrations(ctx, migrationModule, fixed); err != nil {
		t.Fatalf("retry fixed migrations: %v", err)
	}
	if err := control.ApplyMigrations(ctx, migrationModule, fixed); err != nil {
		t.Fatalf("idempotent migration replay: %v", err)
	}
	assertTableExists(t, db, "migration_retry_one", true)
	assertTableExists(t, db, "migration_retry_two", true)
	assertMigrationCount(t, db, migrationModule, 2)

	checksumChanged := fstest.MapFS{
		"0001_one.sql": {Data: append(append([]byte(nil), one...), []byte("\n-- changed")...)},
		"0002_two.sql": fixed["0002_two.sql"],
	}
	if err := control.ApplyMigrations(ctx, migrationModule, checksumChanged); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("checksum mutation error = %v", err)
	}
	if err := control.ApplyMigrations(ctx, migrationModule, fstest.MapFS{"0001_one.sql": fixed["0001_one.sql"]}); err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("removed migration error = %v", err)
	}

	if _, err := db.Exec(`
		UPDATE modary_module_migration
		SET migration_id = ?
		WHERE module_id = ? AND migration_id = ?`,
		migrationModule+"/9999_out_of_order.sql", migrationModule, migrationModule+"/0001_one.sql"); err != nil {
		t.Fatal(err)
	}
	if err := control.ApplyMigrations(ctx, migrationModule, fixed); err == nil || !strings.Contains(err.Error(), "not the current migration prefix") {
		t.Fatalf("migration prefix error = %v", err)
	}
}

func TestMigrationPolicyRejectsTransactionEscapeBeforeAnySideEffect(t *testing.T) {
	for _, test := range []struct {
		name string
		sql  string
	}{
		{name: "commit", sql: `CREATE TABLE escaped(id INTEGER); COMMIT`},
		{name: "end", sql: `CREATE TABLE escaped(id INTEGER); END TRANSACTION`},
		{name: "rollback conflict", sql: `CREATE TABLE escaped(id INTEGER UNIQUE ON CONFLICT ROLLBACK)`},
		{name: "trigger rollback", sql: `
			CREATE TRIGGER escaped_trigger BEFORE INSERT ON first_table
			BEGIN
				SELECT RAISE(ROLLBACK, 'rollback');
			END`},
		{name: "temporary schema", sql: `CREATE TEMP TABLE escaped(id INTEGER)`},
		{name: "qualified temporary schema", sql: `CREATE TABLE temp.escaped(id INTEGER)`},
		{name: "quoted temporary schema", sql: `CREATE TABLE "temp".escaped(id INTEGER)`},
		{name: "single quoted temporary schema", sql: `CREATE TABLE 'temp'.escaped(id INTEGER)`},
		{name: "bracketed temporary schema", sql: `CREATE TABLE [temp].escaped(id INTEGER)`},
		{name: "backtick temporary schema", sql: "CREATE TABLE `temp`.escaped(id INTEGER)"},
		{name: "administrative", sql: `ATTACH DATABASE '__MODARY_ATTACH_PATH__' AS escaped`},
	} {
		t.Run(test.name, func(t *testing.T) {
			attachPath := filepath.Join(t.TempDir(), "escaped.db")
			migrationSQL := strings.ReplaceAll(
				test.sql,
				"__MODARY_ATTACH_PATH__",
				strings.ReplaceAll(attachPath, "'", "''"),
			)
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = db.Close() })
			control, err := databasecontrol.New(&backend{db: db})
			if err != nil {
				t.Fatal(err)
			}
			migrations := fstest.MapFS{
				"0001_first.sql":  {Data: []byte(`CREATE TABLE first_table(id INTEGER)`)},
				"0002_escape.sql": {Data: []byte(migrationSQL)},
			}
			if err := control.ApplyMigrations(context.Background(), "policy-test", migrations); err == nil {
				t.Fatal("unsafe migration was accepted")
			}
			assertTableExists(t, db, "first_table", false)
			assertTableExists(t, db, "escaped", false)
			assertTableExists(t, db, "modary_module_migration", false)
			assertNoTemporarySchemaObjects(t, db)
			assertOnlyMainDatabaseAttached(t, db)
			if _, err := os.Lstat(attachPath); !os.IsNotExist(err) {
				t.Fatalf("rejected migration created attach target %s: %v", attachPath, err)
			}
		})
	}
}

func TestMigrationPolicyAcceptsPersistentTriggerWithCaseExpression(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	control, err := databasecontrol.New(&backend{db: db})
	if err != nil {
		t.Fatal(err)
	}
	migrations := fstest.MapFS{
		"0001_schema.sql": {Data: []byte(`
			CREATE TABLE trigger_item(value INTEGER);
			CREATE TABLE trigger_audit(value INTEGER);
			CREATE TRIGGER trigger_item_audit AFTER INSERT ON trigger_item
			BEGIN
				INSERT INTO trigger_audit(value)
				VALUES (CASE WHEN NEW.value > 0 THEN NEW.value ELSE 0 END);
			END`)},
	}
	if err := control.ApplyMigrations(context.Background(), "trigger-test", migrations); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO trigger_item(value) VALUES (7)`); err != nil {
		t.Fatal(err)
	}
	var value int
	if err := db.QueryRow(`SELECT value FROM trigger_audit`).Scan(&value); err != nil || value != 7 {
		t.Fatalf("trigger audit value = %d, %v", value, err)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, name string, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if got := count == 1; got != want {
		t.Fatalf("table %s exists = %t, want %t", name, got, want)
	}
}

func assertNoTemporarySchemaObjects(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_temp_schema`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected migration left %d temporary schema objects", count)
	}
}

func assertOnlyMainDatabaseAttached(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA database_list`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	databases := make(map[string]string)
	for rows.Next() {
		var (
			sequence int
			name     string
			path     string
		)
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
		databases[name] = path
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if _, exists := databases["main"]; !exists {
		t.Fatalf("rejected migration database list = %v, missing main", databases)
	}
	if path, exists := databases["temp"]; exists && path != "" {
		t.Fatalf("rejected migration temp database path = %q, want empty", path)
	}
	for name := range databases {
		if name != "main" && name != "temp" {
			t.Fatalf("rejected migration left attached database %q: %v", name, databases)
		}
	}
}

func assertMigrationCount(t *testing.T, db *sql.DB, moduleID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM modary_module_migration WHERE module_id = ?`, moduleID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("migration count for %s = %d, want %d", moduleID, count, want)
	}
}
