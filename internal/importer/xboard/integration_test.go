//go:build integration

// Integration suite against real PostgreSQL / MySQL servers (docs/IMPORT_XBOARD.md §9). It is skipped unless
// the admin DSNs are set; every test creates and drops its own database:
//
//	docker run -d --name ggb-it-pg -e POSTGRES_PASSWORD=test -p 127.0.0.1:55491:5432 postgres:16-alpine
//	docker run -d --name ggb-it-mysql -e MYSQL_ROOT_PASSWORD=test -p 127.0.0.1:55492:3306 mysql:8.4
//	GGBOARD_IT_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55491/postgres?sslmode=disable' \
//	GGBOARD_IT_MYSQL_DSN='root:test@tcp(127.0.0.1:55492)/' \
//	go test -tags integration -run Integration ./internal/importer/xboard/
//
// Sources: the SQLite fixtures are loaded into PostgreSQL / MySQL with driver-adjusted DDL and must import
// exactly like the SQLite source. Targets: the SQLite fixtures are imported into PostgreSQL / MySQL targets,
// which checks portability of every write and the PostgreSQL sequence reset.
package xboardimport

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func itName() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "ggb_it_" + hex.EncodeToString(b)
}

// pgDatabase creates a database on the server of GGBOARD_IT_POSTGRES_DSN and returns its URL.
func pgDatabase(t *testing.T) string {
	t.Helper()
	admin := os.Getenv("GGBOARD_IT_POSTGRES_DSN")
	if admin == "" {
		t.Skip("GGBOARD_IT_POSTGRES_DSN not set")
	}
	db, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatal(err)
	}
	name := itName()
	if _, err := db.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		_ = db.Close()
	})
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// mysqlDatabase creates a database on the server of GGBOARD_IT_MYSQL_DSN and returns its go-sql-driver DSN.
func mysqlDatabase(t *testing.T) string {
	t.Helper()
	admin := os.Getenv("GGBOARD_IT_MYSQL_DSN")
	if admin == "" {
		t.Skip("GGBOARD_IT_MYSQL_DSN not set")
	}
	db, err := sql.Open("mysql", admin)
	if err != nil {
		t.Fatal(err)
	}
	name := itName()
	if _, err := db.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP DATABASE IF EXISTS " + name)
		_ = db.Close()
	})
	cfg, err := mysqldriver.ParseDSN(admin)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	return cfg.FormatDSN()
}

var (
	reAutoinc      = regexp.MustCompile(`integer primary key autoincrement not null`)
	reBareVarchar  = regexp.MustCompile(`varchar( not null|,|\))`)
	reBoolColumn   = regexp.MustCompile(`"(\w+)" INTEGER`)
	reCreateTable  = regexp.MustCompile(`(?s)CREATE TABLE "(\w+)" \((.*?)\);\n`)
	reUniqueEmail  = regexp.MustCompile(`CREATE UNIQUE INDEX "email"`)
	keepIntBoolean = map[string]bool{ // legacy boolean() columns that hold more than 0/1
		"v2_plan.reset_traffic_method": true, "v2_server_vless.tls": true, "v2_coupon.type": true, "v2_ticket.level": true,
	}
)

// pgSchema adapts the SQLite DDL; boolean columns are converted after seeding (PostgreSQL rejects 0/1 literals).
func pgSchema(ddl string) (string, []string) {
	ddl = reAutoinc.ReplaceAllString(ddl, "bigserial primary key not null")
	ddl = strings.ReplaceAll(ddl, " datetime", " timestamp(0) without time zone")
	ddl = strings.ReplaceAll(ddl, " double,", " double precision,")
	ddl = strings.ReplaceAll(ddl, " float not null", " double precision not null")
	var alters []string
	for _, m := range reCreateTable.FindAllStringSubmatch(ddl, -1) {
		for _, c := range reBoolColumn.FindAllStringSubmatch(m[2], -1) {
			if keepIntBoolean[m[1]+"."+c[1]] {
				continue
			}
			alters = append(alters, fmt.Sprintf(`ALTER TABLE "%s" ALTER COLUMN "%s" DROP DEFAULT, ALTER COLUMN "%s" TYPE boolean USING "%s" <> 0`, m[1], c[1], c[1], c[1]))
		}
	}
	return ddl, alters
}

func mysqlSchema(ddl string) string {
	ddl = reAutoinc.ReplaceAllString(ddl, "bigint primary key auto_increment not null")
	ddl = reBareVarchar.ReplaceAllString(ddl, "varchar(255)$1")
	ddl = strings.ReplaceAll(ddl, " numeric", " decimal(10,2)")
	ddl = strings.ReplaceAll(ddl, " INTEGER", " tinyint(1)")
	// MySQL's case-insensitive unique index would reject the A@/a@ pair (which is exactly why it cannot occur there)
	return reUniqueEmail.ReplaceAllString(ddl, `CREATE INDEX "email"`)
}

func loadPGSource(t *testing.T, flavor Flavor) string {
	t.Helper()
	dsn := pgDatabase(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ddl, alters := pgSchema(readTestdata(t, string(flavor)+"_schema.sql"))
	for _, s := range append([]string{ddl, readTestdata(t, string(flavor)+"_seed.sql")}, alters...) {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("load %s fixture into postgres: %v", flavor, err)
		}
	}
	return dsn
}

func loadMySQLSource(t *testing.T, flavor Flavor) string {
	t.Helper()
	dsn := mysqlDatabase(t)
	cfg, _ := mysqldriver.ParseDSN(dsn)
	cfg.MultiStatements = true
	cfg.Params = map[string]string{"sql_mode": "'ANSI_QUOTES,STRICT_TRANS_TABLES'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range []string{mysqlSchema(readTestdata(t, string(flavor)+"_schema.sql")), readTestdata(t, string(flavor)+"_seed.sql")} {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("load %s fixture into mysql: %v", flavor, err)
		}
	}
	return dsn
}

// The same fixture imported from PostgreSQL / MySQL must give exactly the SQLite-source result.
func TestIntegrationSources(t *testing.T) {
	for _, flavor := range []Flavor{FlavorLegacy, FlavorUpstream} {
		for _, drv := range []string{DriverPostgres, DriverMySQL} {
			t.Run(string(flavor)+"/"+drv, func(t *testing.T) {
				var src string
				if drv == DriverPostgres {
					src = loadPGSource(t, flavor)
				} else {
					src = "mysql://" + loadMySQLSource(t, flavor)
				}
				stats := func(o *Options) { o.WithStats = true; o.StatsDays = 1000 }
				want, wantRep := mustImport(t, flavor, stats)
				got := testutil.NewTestApp(t)
				gotRep, err := importInto(t, got, src, stats)
				if err != nil {
					t.Fatalf("import from %s: %v", drv, err)
				}
				if gotRep.Flavor != flavor || !reflect.DeepEqual(gotRep.Tables, wantRep.Tables) {
					t.Errorf("summary from %s differs:\n%+v\n%+v", drv, gotRep.Tables, wantRep.Tables)
				}
				if !reflect.DeepEqual(gotRep.Warnings, wantRep.Warnings) {
					t.Errorf("warnings from %s differ:\n%v\n%v", drv, gotRep.Warnings, wantRep.Warnings)
				}
				if a, b := dump(t, got, maskRegenerated), dump(t, want, maskRegenerated); a != b {
					t.Errorf("target from %s differs from the SQLite-source import:\n%s\n---\n%s", drv, a, b)
				}
			})
		}
	}
}

func targetApp(t *testing.T, driver string) *app.App {
	t.Helper()
	var dsn string
	if driver == config.DriverPostgres {
		dsn = pgDatabase(t)
	} else {
		dsn = mysqlDatabase(t)
	}
	return testutil.NewTestApp(t, testutil.WithConfig(func(c *config.Config) {
		c.Database = config.DatabaseConfig{Driver: driver, DSN: dsn}
	}))
}

// Writes are portable and PostgreSQL sequences continue after the imported ids.
func TestIntegrationTargets(t *testing.T) {
	for _, drv := range []string{config.DriverPostgres, config.DriverMySQL} {
		t.Run(drv, func(t *testing.T) {
			a := targetApp(t, drv)
			src := newSource(t, FlavorLegacy)
			dry, err := importInto(t, a, src, func(o *Options) { o.DryRun = true; o.WithStats = true; o.StatsDays = 1000 })
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			targetEmpty(t, a)
			rep, err := importInto(t, a, src, func(o *Options) { o.WithStats = true; o.StatsDays = 1000 })
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			if !reflect.DeepEqual(dry.Tables, rep.Tables) {
				t.Errorf("dry run differs from the real run")
			}
			if n := count(t, a, &model.User{}); n != 14 {
				t.Errorf("users = %d", n)
			}
			for _, id := range []uint64{1, 2, 3, 4} {
				if u := loadUser(t, a, id); !auth.VerifyPassword(u.PasswordHash, "password123") {
					t.Errorf("user %d password", id)
				}
			}
			if n := loadNode(t, a, 12); n.ParentID == nil || *n.ParentID != 11 || n.ProtocolSettings.String("network") != "tcp" {
				t.Errorf("node 12: %+v", n)
			}
			u := model.User{Email: "new@example.com", PasswordHash: "!", UUID: "20000000-0000-4000-8000-000000000001", Token: "b0000000000000000000000000000001"}
			if err := a.DB.Create(&u).Error; err != nil || u.ID != 15 {
				t.Errorf("next user id = %d (%v), want 15", u.ID, err)
			}
			nd := model.Node{Type: model.NodeVmess, Name: "new", Host: "new.example.com", Port: "1", Rate: 1}
			if err := a.DB.Create(&nd).Error; err != nil || nd.ID != 13 {
				t.Errorf("next node id = %d (%v), want 13", nd.ID, err)
			}
			var su []model.StatUser
			a.DB.Find(&su)
			if len(su) != 1 || su[0].U != 1200 || su[0].Rate != 0.88 {
				t.Errorf("stat_users: %+v", su)
			}
			if _, err := importInto(t, a, src, func(o *Options) { o.Force = true }); err != nil {
				t.Fatalf("--force: %v", err)
			}
			if n := count(t, a, &model.User{}); n != 14 {
				t.Errorf("users after --force = %d", n)
			}
			if a.Settings.String("server_token") != "xboard-example-server-token" {
				t.Error("settings not imported")
			}
			up := targetApp(t, drv)
			if _, err := importInto(t, up, newSource(t, FlavorUpstream)); err != nil {
				t.Fatalf("upstream import: %v", err)
			}
			if n := count(t, up, &model.Node{}); n != 12 {
				t.Errorf("upstream nodes = %d", n)
			}
		})
	}
}
