package sqlpolicy

import "testing"

func TestValidateMigrationScriptAcceptsDurableDDLAndDML(t *testing.T) {
	for _, script := range []string{
		`CREATE TABLE item(id INTEGER PRIMARY KEY); CREATE INDEX item_id ON item(id); INSERT INTO item(id) VALUES (1)`,
		`-- BEGIN; COMMIT; ROLLBACK
		 CREATE TABLE "ROLLBACK" ([COMMIT] TEXT DEFAULT 'END; SAVEPOINT');`,
		`CREATE TRIGGER item_guard
		 BEFORE UPDATE ON item
		 WHEN CASE WHEN NEW.id > 0 THEN 1 ELSE 0 END = 1
		 BEGIN
		   UPDATE item SET id = CASE WHEN id > 1 THEN id ELSE 1 END;
		   INSERT INTO audit(value) VALUES ('COMMIT; ROLLBACK');
		 END;
		 CREATE INDEX item_guard_index ON item(id)`,
		`CREATE TABLE "temp" (value TEXT DEFAULT 'temp.item')`,
	} {
		if err := ValidateMigrationScript(script, 1<<20); err != nil {
			t.Fatalf("ValidateMigrationScript() error = %v\n%s", err, script)
		}
	}
}

func TestValidateMigrationScriptRejectsTransactionAndEphemeralSQL(t *testing.T) {
	for _, script := range []string{
		`BEGIN; CREATE TABLE item(id INTEGER); COMMIT`,
		`CREATE TABLE item(id INTEGER); END TRANSACTION`,
		`CREATE TABLE item(id INTEGER); ROLLBACK TO SAVEPOINT nested`,
		`CREATE TABLE item(id INTEGER); SAVEPOINT nested`,
		`CREATE TABLE item(id INTEGER); RELEASE SAVEPOINT nested`,
		`CREATE TABLE item(id INTEGER UNIQUE ON CONFLICT ROLLBACK)`,
		`CREATE TRIGGER item_guard BEFORE INSERT ON item BEGIN SELECT RAISE(ROLLBACK, 'denied'); END`,
		`CREATE TRIGGER item_guard BEFORE INSERT ON item BEGIN SELECT 1; END; COMMIT`,
		`CREATE TEMP TABLE item(id INTEGER)`,
		`CREATE TEMPORARY TRIGGER item_guard BEFORE INSERT ON item BEGIN SELECT 1; END`,
		`CREATE TABLE temp.ephemeral(id INTEGER)`,
		`CREATE TABLE "temp".ephemeral(id INTEGER)`,
		`CREATE TABLE 'temp'.ephemeral(id INTEGER)`,
		"CREATE TABLE `temp`.ephemeral(id INTEGER)",
		`CREATE TABLE [temp].ephemeral(id INTEGER)`,
		`CREATE INDEX temp.ephemeral_index ON item(id)`,
		`ALTER TABLE "temp".ephemeral ADD COLUMN value INTEGER`,
		`DROP TABLE [temp].ephemeral`,
		`INSERT INTO temp.ephemeral(id) VALUES (1)`,
		`SET search_path = public`,
		`ATTACH DATABASE 'other.db' AS other`,
		`VACUUM`,
		`WITH value AS (SELECT 1) INSERT INTO item SELECT * FROM value`,
		`REPLACE INTO item(id) VALUES (1)`,
		`CREATE TABLE item(id INTEGER);; CREATE TABLE other(id INTEGER)`,
		`CREATE TABLE item(id INTEGER); 'not a statement'`,
		`/* unterminated`,
		`CREATE TABLE item(value TEXT DEFAULT 'unterminated)`,
	} {
		if err := ValidateMigrationScript(script, 1<<20); err == nil {
			t.Fatalf("ValidateMigrationScript() accepted:\n%s", script)
		}
	}
}
