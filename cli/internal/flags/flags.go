// Package flags holds shared CLI flag state accessible by all command packages
// without creating circular imports between cmd and its subpackages.
package flags

// JSON is set by the --json persistent flag on the root command.
// All subcommands read this to determine output format.
var JSON bool
