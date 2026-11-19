// Package migrations embeds the goose SQL migrations.
//
// Rules for adding migrations: never edit an existing file; add a new file
// named NNNNN_description.sql with the next free number.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
