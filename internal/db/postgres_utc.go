package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
)

// pgx decodes timestamptz values with time.Unix, i.e. in the process's local time zone, while SQLite (utcConnector)
// and MySQL (loc=UTC) return UTC. The rest of the code relies on UTC: API timestamps are RFC3339 with "Z", and
// calendar arithmetic on stored times (AddDate on expired_at when an order renews a plan) runs in the time's own
// location. Under TZ=Asia/Shanghai a PostgreSQL panel would answer "+08:00" and add months in local time, so every
// PostgreSQL connection scans timestamptz in UTC instead (the instant is unchanged).

// openPostgres opens the PostgreSQL pool for Open. It is built here rather than by gorm.io/driver/postgres so that
// the UTC codec is the only AfterConnect hook: the driver installs its own hook (replacing any other) when the DSN
// carries TimeZone=, which is still sent to the server as a run-time parameter.
func openPostgres(dsn string) (*sql.DB, error) {
	pc, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: invalid postgres dsn: %w", err)
	}
	return stdlib.OpenDB(*pc, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		scanTimestamptzInUTC(conn.TypeMap())
		return nil
	})), nil
}

// scanTimestamptzInUTC makes m return timestamptz values in UTC.
func scanTimestamptzInUTC(m *pgtype.Map) {
	m.RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
}
