// Package database defines narrow provider-neutral SQL contracts for ordinary
// business repositories and governed operation handlers. Framework-owned
// implementations validate public queries and mutations. Raw connections,
// commit, rollback, migration, and administration remain internal.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package database
