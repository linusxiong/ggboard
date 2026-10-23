package cli

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/console"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/static"
)

// printSummary writes the startup summary of `serve` (docs/DEPLOY.md §9) once the listener is open: version,
// local URL, site URL, database, data directory, logs, frontend, plugins, jobs and first-run hints. It never prints
// secrets: the database line has the driver with the SQLite file or the server host and database name, never a
// user name, password or query parameters.
func printSummary(ctx context.Context, p *console.Printer, a *app.App, addr net.Addr) {
	ready := time.Since(a.StartedAt).Round(time.Millisecond)
	local, listen := localURL(addr)

	site := p.Faint("not set (Admin → Settings → Site)")
	if u := strings.TrimSpace(a.Settings.String("app_url")); u != "" {
		site = p.Accent(u)
	}
	logs := a.Cfg.Log.Level + " · " + a.Cfg.Log.Format
	frontend, frontendOK := frontendState(a)
	if !frontendOK {
		frontend = p.Caution(frontend)
	}

	var enabledPayments, admins int64
	db := a.DB.WithContext(ctx)
	_ = db.Model(&model.Payment{}).Where(map[string]any{"enabled": true}).Count(&enabledPayments).Error
	_ = db.Model(&model.User{}).Where(map[string]any{"is_admin": true}).Count(&admins).Error
	notifiers := p.Faint("off")
	if names := notify.ActiveNames(a); len(names) > 0 {
		notifiers = strings.Join(names, ", ")
	}

	listenNote := ""
	if listen != "" {
		listenNote = " " + p.Faint("(listening on "+listen+")")
	}
	fields := []console.Field{
		{Key: "Local", Value: p.Accent(local) + listenNote, Mark: console.SymbolArrow},
		{Key: "Site", Value: site, Mark: console.SymbolArrow},
		{Key: "Database", Value: p.Text(databaseSummary(a.Cfg.Database))},
		{Key: "Data", Value: p.Text(absPath(a.Cfg.DataDir))},
		{Key: "Logs", Value: logs},
		{Key: "Frontend", Value: frontend},
		{Key: "Payments", Value: fmt.Sprintf("%d gateways · %d enabled", len(payment.Names()), enabledPayments)},
		{Key: "Notify", Value: notifiers},
		{Key: "Jobs", Value: fmt.Sprintf("%d scheduled", len(a.Scheduler.Status()))},
	}

	if p.Pretty() {
		p.Println()
		p.Println("  " + p.Bold("GG Bond") + " " + p.Accent(a.Version) + "  " + p.Faint("ready in "+ready.String()))
		p.Println()
	} else {
		p.Println("GG Bond " + p.Text(a.Version) + " ready in " + ready.String())
	}
	p.Fields(2, fields)
	if admins == 0 {
		if p.Pretty() {
			p.Println()
		}
		p.Indented(2).Warning("No administrator yet. Create one with: " + p.Accent("ggboard admin create --email you@example.com"))
	}
	if p.Pretty() {
		p.Println()
	}
}

// localURL is the URL to open on this machine for the listener address, plus the address itself when the listener
// accepts every interface (":8080", "0.0.0.0:8080", "[::]:8080").
func localURL(addr net.Addr) (string, string) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String() + "/", ""
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		return "http://localhost:" + port + "/", addr.String()
	}
	return "http://" + net.JoinHostPort(host, port) + "/", ""
}

// frontendState describes where the SPA comes from and whether it is usable.
func frontendState(a *app.App) (string, bool) {
	fsys, err := static.FS(a)
	if err != nil {
		return "frontend.dir is not a directory", false
	}
	where := "embedded"
	if a.Cfg.Frontend.Dir != "" {
		where = absPath(a.Cfg.Frontend.Dir)
	}
	if _, err := fs.Stat(fsys, "index.html"); err != nil {
		return where + ": not built (make web)", false
	}
	return where, true
}

// databaseSummary is "driver · location" without credentials: the SQLite file, or host and database name.
func databaseSummary(c config.DatabaseConfig) string {
	switch c.Driver {
	case config.DriverSQLite:
		path, _, _ := strings.Cut(strings.TrimPrefix(c.DSN, "file:"), "?")
		if path == "" || path == ":memory:" || strings.HasPrefix(path, ":memory:") {
			return c.Driver + " · in memory"
		}
		return c.Driver + " · " + absPath(path)
	case config.DriverMySQL:
		if m, err := mysqldriver.ParseDSN(c.DSN); err == nil {
			return c.Driver + " · " + m.Addr + "/" + m.DBName
		}
	case config.DriverPostgres:
		if host, name := postgresTarget(c.DSN); host != "" || name != "" {
			return c.Driver + " · " + host + "/" + name
		}
	}
	return c.Driver
}

// postgresTarget extracts host[:port] and the database name from a URL or key=value DSN.
func postgresTarget(dsn string) (host, name string) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", ""
		}
		return u.Host, strings.TrimPrefix(u.Path, "/")
	}
	var port string
	for _, kv := range strings.Fields(dsn) {
		k, v, _ := strings.Cut(kv, "=")
		v = strings.Trim(v, `'`)
		switch k {
		case "host":
			host = v
		case "port":
			port = v
		case "dbname":
			name = v
		}
	}
	if host != "" && port != "" {
		host = net.JoinHostPort(host, port)
	}
	return host, name
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
