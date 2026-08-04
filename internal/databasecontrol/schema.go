package databasecontrol

import (
	"crypto/sha256"
	"encoding/binary"
)

// SchemaProfileTable is the framework-owned physical-schema role marker used
// by official database components to reject incompatible reuse.
const SchemaProfileTable = "modary_schema_profile"

const (
	SchemaRoleApplication = "application"
	SchemaRoleQueue       = "queue"
)

// SchemaAdvisoryLockKey gives every official component the same lock identity
// for a physical schema. PostgreSQL scopes advisory locks to one database, so
// the driver and schema name are sufficient within a connection.
func SchemaAdvisoryLockKey(driver, schema string) int64 {
	digest := sha256.Sum256([]byte("modary:database-schema:" + driver + ":" + schema))
	key := int64(binary.BigEndian.Uint64(digest[:8]))
	if key == 0 {
		return 1
	}
	return key
}
