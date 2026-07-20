// Command dropcrate is an expiring object-storage and file-sharing API.
//
// dropcrate accepts file uploads over HTTP, persists the bytes in an
// S3-compatible object store, keeps metadata in MySQL, caches lookups in
// Redis, and hands out short download links that can be set to expire.
package main

import (
	"embed"

	"github.com/IgliHoxha/dropcrate/cmd"
)

// migrationsFS bundles the SQL schema into the binary so `dropcrate migrate`
// works from a single self-contained executable.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	cmd.MigrationsFS = migrationsFS
	cmd.Execute()
}
