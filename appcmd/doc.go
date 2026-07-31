// Package appcmd provides consumer-owned application commands without exposing
// Modary lifecycle or Action handler internals.
//
// Run accepts a fallible DefinitionProvider for runtime commands. Help, version,
// and invalid syntax use the pure Metadata in Options without invoking it.
// Serve and RunAction are lower-level entry points for an assembled Definition.
//
// Stability: alpha. Until Modary reaches v1, exported Go APIs and command
// syntax may change between minor releases. Consumers should pin an exact
// module version and review release notes before upgrading.
package appcmd
