// Package migrations embeds the SQL migration files so the migrate command
// carries its schema with it.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
