package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
)

// The pure-Go SQLite driver writes time.Time parameters using the value's own UTC offset. Because SQLite
// compares datetimes as text, mixing local and UTC values would make `expired_at > ?` wrong. This connector
// converts every time parameter to UTC before it reaches the driver, so all stored datetimes share one offset.

func openSQLiteUTC(dsn string) (*sql.DB, error) {
	probe, err := sql.Open(sqlite.DriverName, "")
	if err != nil {
		return nil, fmt.Errorf("db: sqlite driver: %w", err)
	}
	drv := probe.Driver()
	_ = probe.Close()
	return sql.OpenDB(&utcConnector{drv: drv, dsn: dsn}), nil
}

type utcConnector struct {
	drv driver.Driver
	dsn string
}

func (c *utcConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &utcConn{Conn: conn}, nil
}

func (c *utcConnector) Driver() driver.Driver { return c.drv }

type utcConn struct{ driver.Conn }

var (
	_ driver.NamedValueChecker  = (*utcConn)(nil)
	_ driver.ConnBeginTx        = (*utcConn)(nil)
	_ driver.ConnPrepareContext = (*utcConn)(nil)
	_ driver.ExecerContext      = (*utcConn)(nil)
	_ driver.QueryerContext     = (*utcConn)(nil)
	_ driver.Pinger             = (*utcConn)(nil)
	_ driver.SessionResetter    = (*utcConn)(nil)
	_ driver.Validator          = (*utcConn)(nil)
)

// CheckNamedValue applies the default conversion (Valuer, pointers, integer kinds) and then forces UTC.
func (c *utcConn) CheckNamedValue(nv *driver.NamedValue) error {
	v, err := driver.DefaultParameterConverter.ConvertValue(nv.Value)
	if err != nil {
		return err
	}
	if t, ok := v.(time.Time); ok {
		v = t.UTC()
	}
	nv.Value = v
	return nil
}

func (c *utcConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.Conn.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck // fallback for drivers without BeginTx
}

func (c *utcConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return p.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func (c *utcConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if e, ok := c.Conn.(driver.ExecerContext); ok {
		return e.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *utcConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if q, ok := c.Conn.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *utcConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *utcConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *utcConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}
