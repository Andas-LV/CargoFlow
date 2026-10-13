// Package migrations embeds the auth service's goose SQL migrations into the binary.
package migrations

import "embed"

// FS holds the *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
