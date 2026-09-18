// Package migrations exposes the database migrations to the application
// binary. Keeping them in the binary lets production run the exact migrations
// that belong to a release without installing Go, goose, or a source checkout
// on the server.
package migrations

import "embed"

// FS contains every versioned goose migration in this directory.
//
//go:embed *.sql
var FS embed.FS
