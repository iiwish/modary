// Package database defines the narrow SQL value and capability contracts
// available to consumer Action handlers. The framework-owned implementation
// validates public queries and mutations. Privileged backend construction,
// migration, administration, and transaction ownership remain internal.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package database
