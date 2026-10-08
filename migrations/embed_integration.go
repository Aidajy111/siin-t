//go:build integration

package migrations

import "embed"

// Files contains the same SQL migrations used by Docker Compose.
//
//go:embed *.up.sql
var Files embed.FS
