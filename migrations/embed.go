// Package migrations embeds RADAR's SQL migration files so the radar binary
// can apply them without depending on an external migrate CLI or a
// filesystem path at runtime.
package migrations

import "embed"

// FS holds the embedded *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
