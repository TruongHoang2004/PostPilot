package migrations

import "embed"

// FS embeds all SQL migration files directly into the Go binary at compile time.
// This ensures runtime containers and binaries have no external file path dependencies.
//
//go:embed *.sql
var FS embed.FS
