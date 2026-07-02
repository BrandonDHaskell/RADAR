// Package templates embeds RADAR's digest templates so the radar binary
// can render them without depending on a filesystem path at runtime.
package templates

import "embed"

// FS holds the embedded *.tmpl template files.
//
//go:embed *.tmpl
var FS embed.FS
