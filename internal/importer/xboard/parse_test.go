package xboardimport

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/auth"
)

func TestFlexScanners(t *testing.T) {
	ints := []struct {
		in   any
		want int64
		ok   bool
	}{
		{int64(5), 5, true}, {true, 1, true}, {false, 0, true}, {[]byte(" 42 "), 42, true}, {"7", 7, true},
		{"2.6", 3, true}, {3.4, 3, true}, {nil, 0, false}, {"", 0, false}, {"abc", 0, false},
	}
	for _, c := range ints {
		got, ok := flexInt(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("flexInt(%#v) = %d,%v; want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
	for _, v := range []any{1, int64(1), "1", "true", "on", "yes", true, []byte("1"), "TRUE"} {
		if !flexBool(v) {
			t.Errorf("flexBool(%#v) = false", v)
		}
	}
	for _, v := range []any{0, "0", "", "false", "off", "no", nil, false, []byte("")} {
		if flexBool(v) {
			t.Errorf("flexBool(%#v) = true", v)
		}
	}
	if f, ok := flexFloat("19.99"); !ok || f != 19.99 {
		t.Errorf("flexFloat = %v %v", f, ok)
	}
	if s, ok := flexStr("abc       "); !ok || s != "abc" {
		t.Errorf("flexStr char(n) padding = %q", s)
	}
	if cents(9.99) != 999 || cents(99.9) != 9990 || cents(19.99) != 1999 {
		t.Error("cents rounding")
	}
	if nz(0) != nil || nz(nil) != nil || *nz("5") != 5 {
		t.Error("nz")
	}
}

func TestParseIDList(t *testing.T) {
	cases := []struct {
		in   any
		want []uint64
		bad  int
	}{
		{`["1","2"]`, []uint64{1, 2}, 0},
		{`[1]`, []uint64{1}, 0},
		{`{"0":"1","2":"3"}`, []uint64{1, 3}, 0},
		{`{"5":"9","1":"4"}`, []uint64{9, 4}, 0}, // document order, not key order
		{`"1,2"`, []uint64{1, 2}, 0},
		{`1,2`, []uint64{1, 2}, 0},
		{"1\n2|3", []uint64{1, 2, 3}, 0},
		{`3`, []uint64{3}, 0},
		{`"3"`, []uint64{3}, 0},
		{nil, nil, 0},
		{``, nil, 0},
		{`[]`, nil, 0},
		{`["1","x",0,-2,"1"]`, []uint64{1}, 3},
	}
	for _, c := range cases {
		got, bad, err := parseIDList(c.in)
		if err != nil {
			t.Errorf("parseIDList(%#v) error %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) || len(bad) != c.bad {
			t.Errorf("parseIDList(%#v) = %v bad %v; want %v bad %d", c.in, got, bad, c.want, c.bad)
		}
	}
	if _, _, err := parseIDList(`[1,2`); err == nil {
		t.Error("malformed JSON must report an error")
	}
}

func TestParseStringList(t *testing.T) {
	cases := []struct {
		in   any
		want []string
	}{
		{`["a"," b ",""]`, []string{"a", "b"}},
		{`{"0":"domain:a.example.com","2":"domain:b.example.com"}`, []string{"domain:a.example.com", "domain:b.example.com"}},
		{"geosite:netflix\r\ndomain:x.example.com\n", []string{"geosite:netflix", "domain:x.example.com"}},
		{"regexp:(a|b)\\.example\\.com", []string{"regexp:(a|b)\\.example\\.com"}},
		{"a.example.com, b.example.com", []string{"a.example.com", "b.example.com"}},
		{nil, []string{}},
	}
	for _, c := range cases {
		got, _, err := parseStringList(c.in)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseStringList(%#v) = %#v, %v; want %#v", c.in, got, err, c.want)
		}
	}
}

func TestServerKey(t *testing.T) {
	if got := serverKey(1700000000, 16); got != "MjQ5MjBkZWNmODNmNjk2MA==" {
		t.Errorf("K16 = %s", got)
	}
	if got := serverKey(1700000000, 32); got != "MjQ5MjBkZWNmODNmNjk2MGI4MGY3MzdhMjhlZWFmYzE=" {
		t.Errorf("K32 = %s", got)
	}
}

func TestMapPassword(t *testing.T) {
	cases := []struct{ pw, algo, salt, wantPrefix, kind string }{
		{"$2y$10$JSVs9TpWmq34RiU1ShgOYu/q/tsdYkEto6AuJr5.WOgSkUT8QiiJK", "", "", "$2y$", hashBcrypt},
		{"482c811da5d5b4bc6d497ffa98491e38", "md5", "", "xb$md5$$", hashLegacy},
		{"ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f", "sha256 ", "", "xb$sha256$$", hashLegacy},
		{"1b84a6e599feeab25ace9f402aaa4858", "md5salt", "abc$12", "xb$md5salt$abc$12$", hashLegacy},
		{"d65cf2f6dad004d515d5ef2d41f7d99bf493e8dece1b17aacbc6c29a0e2fd00b", "sha256salt", "NaCl", "xb$sha256salt$NaCl$", hashLegacy},
		{"cbfdac6008f9cab4083784cbd1874f76618d2a97", "sha1", "", unusableHash, hashUnusable},
		{"plaintext", "", "", unusableHash, hashUnusable},
		{"nothex", "md5", "", unusableHash, hashUnusable},
	}
	for _, c := range cases {
		got, kind := mapPassword(c.pw, c.algo, c.salt)
		if !strings.HasPrefix(got, c.wantPrefix) || kind != c.kind {
			t.Errorf("mapPassword(%q,%q) = %q,%s; want prefix %q,%s", c.pw, c.algo, got, kind, c.wantPrefix, c.kind)
		}
		if kind != hashUnusable && !auth.VerifyPassword(got, "password123") {
			t.Errorf("%s hash does not verify password123", c.algo)
		}
		if auth.VerifyPassword(got, "wrong") {
			t.Errorf("%s hash verifies a wrong password", c.algo)
		}
	}
}

func TestParseDSN(t *testing.T) {
	cases := []struct {
		dsn, driver string
		wantDriver  string
		contains    []string
		display     string
	}{
		{"mysql://xboard:s3cr3t@127.0.0.1:3306/xboard?parseTime=true", "", DriverMySQL,
			[]string{"tcp(127.0.0.1:3306)/xboard", "interpolateParams=true", "charset=utf8mb4"}, "mysql 127.0.0.1:3306/xboard"},
		{"mysql://xboard:s3cr3t@tcp(127.0.0.1:3306)/xboard?parseTime=true&loc=Local", "", DriverMySQL,
			[]string{"tcp(127.0.0.1:3306)/xboard", "interpolateParams=true"}, "mysql 127.0.0.1:3306/xboard"},
		{"mysql://xboard:s3cr3t@db.example.com/xboard", "", DriverMySQL, []string{"tcp(db.example.com:3306)/xboard"}, "mysql db.example.com:3306/xboard"},
		{"xboard:s3cr3t@tcp(127.0.0.1:3306)/xboard", "mysql", DriverMySQL, []string{"tcp(127.0.0.1:3306)/xboard"}, "mysql 127.0.0.1:3306/xboard"},
		{"postgres://xboard:s3cr3t@127.0.0.1:5432/xboard?sslmode=disable", "", DriverPostgres,
			[]string{"postgres://xboard:s3cr3t@127.0.0.1:5432/xboard?sslmode=disable"}, "postgres 127.0.0.1:5432/xboard"},
		{"postgresql://u@h/db", "", DriverPostgres, nil, "postgres h/db"},
		{"sqlite:///srv/xboard/database.sqlite", "", DriverSQLite, []string{"file:/srv/xboard/database.sqlite?mode=ro", "busy_timeout"}, "sqlite /srv/xboard/database.sqlite"},
	}
	for _, c := range cases {
		got, err := ParseDSN(c.dsn, c.driver)
		if err != nil {
			t.Errorf("ParseDSN(%q): %v", c.dsn, err)
			continue
		}
		if got.Driver != c.wantDriver || got.Display != c.display {
			t.Errorf("ParseDSN(%q) = %s %q; want %s %q", c.dsn, got.Driver, got.Display, c.wantDriver, c.display)
		}
		for _, s := range c.contains {
			if !strings.Contains(got.DSN, s) {
				t.Errorf("ParseDSN(%q).DSN = %q; missing %q", c.dsn, got.DSN, s)
			}
		}
		if strings.Contains(got.Display, "s3cr3t") {
			t.Errorf("display leaks the password: %q", got.Display)
		}
		if c.wantDriver == DriverMySQL && (strings.Contains(got.DSN, "parseTime=true") || strings.Contains(got.DSN, "loc=")) {
			t.Errorf("mysql DSN must force parseTime=false and drop loc: %q", got.DSN)
		}
	}
	for _, bad := range []struct{ dsn, driver string }{{"", ""}, {"xboard.sqlite", ""}, {"sqlite:///x", "mysql"}, {"x", "oracle"}, {"mysql://u:p@h:3306/", ""}} {
		if _, err := ParseDSN(bad.dsn, bad.driver); !errors.Is(err, ErrUsage) {
			t.Errorf("ParseDSN(%q, %q) = %v; want a usage error", bad.dsn, bad.driver, err)
		}
	}
}

func TestDetectFlavor(t *testing.T) {
	base := map[string]bool{"v2_user": true, "v2_settings": true}
	with := func(extra ...string) map[string]bool {
		m := map[string]bool{}
		for k := range base {
			m[k] = true
		}
		for _, e := range extra {
			m[e] = true
		}
		return m
	}
	if f, err := detectFlavor(with("v2_server_vmess")); err != nil || f != FlavorLegacy {
		t.Errorf("legacy: %v %v", f, err)
	}
	if f, err := detectFlavor(with("v2_server", "v2_server_group")); err != nil || f != FlavorUpstream {
		t.Errorf("upstream: %v %v", f, err)
	}
	if _, err := detectFlavor(with("v2_server", "v2_server_trojan")); !errors.Is(err, ErrHalfMigrated) {
		t.Errorf("both: %v", err)
	}
	if _, err := detectFlavor(with()); !errors.Is(err, ErrNotXboard) {
		t.Errorf("neither: %v", err)
	}
	if _, err := detectFlavor(map[string]bool{"v2_server": true}); !errors.Is(err, ErrNotXboard) {
		t.Errorf("no v2_user: %v", err)
	}
}

func TestDatetime(t *testing.T) {
	loc := shanghai()
	for _, v := range []any{"2023-11-15 06:13:20", []byte("2023-11-15 06:13:20"), time.Date(2023, 11, 15, 6, 13, 20, 0, time.UTC)} {
		got, ok := datetime(v, loc)
		if !ok || got.Unix() != 1700000000 {
			t.Errorf("datetime(%#v) = %v %v; want unix 1700000000", v, got, ok)
		}
	}
	if _, ok := datetime(nil, loc); ok {
		t.Error("NULL datetime must not parse")
	}
}
