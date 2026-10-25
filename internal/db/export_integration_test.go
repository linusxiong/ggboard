//go:build integration

package db

import "testing"

// ITDatabase exposes itDatabase (a fresh, dropped-afterwards database on the server of GGBOARD_IT_POSTGRES_DSN /
// GGBOARD_IT_MYSQL_DSN) to the external integration tests of package db_test.
func ITDatabase(t *testing.T, driver string) string { return itDatabase(t, driver) }
