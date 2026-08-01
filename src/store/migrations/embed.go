// Package migrations holds the SQL schema migrations that store.Open applies
// in order. Each file is one numbered migration (0001_init.sql first, then
// 0002_*.sql, and so on); the version recorded in schema_migrations is the
// numeric prefix of the filename.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
