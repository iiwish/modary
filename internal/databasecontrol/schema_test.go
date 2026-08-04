package databasecontrol

import "testing"

func TestSchemaAdvisoryLockKeyUsesDriverAndPhysicalSchema(t *testing.T) {
	first := SchemaAdvisoryLockKey("postgres", "application")
	if first == 0 || first != SchemaAdvisoryLockKey("postgres", "application") {
		t.Fatalf("PostgreSQL application schema lock key is not stable: %d", first)
	}
	if first == SchemaAdvisoryLockKey("postgres", "queue") {
		t.Fatal("different physical schemas share one lock key")
	}
	if first == SchemaAdvisoryLockKey("mysql", "application") {
		t.Fatal("different database drivers share one lock key")
	}
}
