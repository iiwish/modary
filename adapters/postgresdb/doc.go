// Package postgresdb provides the lightweight general PostgreSQL component.
// Selecting it creates one application schema and a provider-neutral Store;
// River, task workers, governed Action persistence, audit, and MCP are absent.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package postgresdb
