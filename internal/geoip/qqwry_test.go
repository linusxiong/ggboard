package geoip_test

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"testing"

	"github.com/linusxiong/ggboard/internal/geoip"
	"github.com/linusxiong/ggboard/internal/geoip/geoiptest"
)

func sampleDB(t *testing.T) *geoip.DB {
	t.Helper()
	db, err := geoip.Parse(geoiptest.Build(geoiptest.Sample, geoiptest.SampleVersion))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestParseSynthetic(t *testing.T) {
	db := sampleDB(t)
	if db.Count() != len(geoiptest.Sample)+1 || db.Version() != geoiptest.SampleVersion {
		t.Fatalf("count = %d version = %q", db.Count(), db.Version())
	}
	cases := []struct{ ip, country, area, location string }{
		// plain strings
		{"0.0.0.1", "IANA", "保留地址(This_network)", "保留地址"},
		// mode 2 (shared country) with a plain area
		{"10.1.2.3", "IANA", "局域网IP(Private-Use)", "局域网IP"},
		// mode 1 → mode 2 (the same country + area as 10.0.0.0/8)
		{"192.168.1.1", "IANA", "局域网IP(Private-Use)", "局域网IP"},
		{"127.0.0.1", "IANA", "本机地址/环回地址(Loopback)", "本机地址"},
		// mode 1 → plain (the same as 8.8.4.4)
		{"8.8.8.8", "美国–加利福尼亚州–圣克拉拉–山景城", "谷歌公司DNS服务器", "美国 加利福尼亚州 圣克拉拉 山景城 谷歌公司DNS服务器"},
		{"1.1.1.1", "澳大利亚", "APNIC/CloudFlare公共DNS服务器", "澳大利亚 APNIC/CloudFlare公共DNS服务器"},
		{"114.114.114.114", "中国–江苏–南京", "南京信风网络科技有限公司GreatbitDNS服务器", "中国 江苏 南京 南京信风网络科技有限公司GreatbitDNS服务器"},
		// mode 2 with an area redirect ("电信" was written for 114.114.114.115)
		{"203.0.113.9", "中国–广东–深圳", "电信", "中国 广东 深圳 电信"},
		{"203.0.113.200", "中国–广东–深圳", "CZ88.NET", "中国 广东 深圳"},
		{"223.5.5.5", "中国–浙江–杭州", "阿里巴巴anycast公共DNS", "中国 浙江 杭州 阿里巴巴anycast公共DNS"},
		{"223.5.5.200", "中国–北京–北京", "联通/北京百度网讯科技有限公司联通节点", "中国 北京 联通"},
		{"255.255.255.255", "纯真网络", "2026年10月21日IP数据", ""},
	}
	for _, c := range cases {
		country, area, ok := db.Lookup(netip.MustParseAddr(c.ip))
		if !ok || country != c.country || area != c.area {
			t.Errorf("Lookup(%s) = %q, %q, %v; want %q, %q", c.ip, country, area, ok, c.country, c.area)
		}
		if got := db.Locate(netip.MustParseAddr(c.ip)); got != c.location {
			t.Errorf("Locate(%s) = %q, want %q", c.ip, got, c.location)
		}
	}
	// A gap between ranges, an address below the first range, IPv6 and IPv4-mapped IPv6.
	for _, ip := range []string{"198.51.100.7", "0.0.0.0", "2001:db8::1"} {
		if _, _, ok := db.Lookup(netip.MustParseAddr(ip)); ok {
			t.Errorf("Lookup(%s) found a range", ip)
		}
	}
	if got := db.Locate(netip.MustParseAddr("::ffff:203.0.113.9")); got != "中国 广东 深圳 电信" {
		t.Errorf("IPv4-mapped = %q", got)
	}
	// Without the release stamp there is no version.
	if db, err := geoip.Parse(geoiptest.Build(geoiptest.Sample, "")); err != nil || db.Version() != "" {
		t.Fatalf("no stamp: %v %q", err, db.Version())
	}
}

func TestParseRejectsCorruptFiles(t *testing.T) {
	good := geoiptest.Build(geoiptest.Sample, geoiptest.SampleVersion)
	first := binary.LittleEndian.Uint32(good[0:4])
	mutate := func(fn func(b []byte) []byte) []byte { return fn(append([]byte(nil), good...)) }
	cases := map[string][]byte{
		"empty":         {},
		"html":          []byte("<!DOCTYPE html><html><body>rate limited</body></html>"),
		"truncated":     good[:len(good)-3],
		"index outside": mutate(func(b []byte) []byte { binary.LittleEndian.PutUint32(b[4:8], uint32(len(b))); return b }),
		"misaligned": mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[4:8], binary.LittleEndian.Uint32(b[4:8])-1)
			return b
		}),
		"unsorted": mutate(func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[first+7:], 0xfffffff0) // second start IP after the third
			return b
		}),
		"record offset": mutate(func(b []byte) []byte {
			b[first+4], b[first+5], b[first+6] = 0xff, 0xff, 0x7f
			return b
		}),
		"dangling redirect": mutate(func(b []byte) []byte {
			// The first record's location becomes a mode-2 redirect past the end of the file.
			rec := uint32(b[first+4]) | uint32(b[first+5])<<8 | uint32(b[first+6])<<16
			b[rec+4], b[rec+5], b[rec+6], b[rec+7] = 0x02, 0xff, 0xff, 0xff
			return b
		}),
		"unterminated": mutate(func(b []byte) []byte {
			// No NUL anywhere: every string runs to the end of the file.
			for i := 8; i < int(first); i++ {
				if b[i] == 0 {
					b[i] = 'x'
				}
			}
			return b
		}),
	}
	for name, data := range cases {
		if _, err := geoip.Parse(data); !errors.Is(err, geoip.ErrFormat) {
			t.Errorf("%s: err = %v, want ErrFormat", name, err)
		}
	}
}

func TestFormatLocation(t *testing.T) {
	cases := []struct{ country, area, want string }{
		{"中国–广东–深圳", "电信", "中国 广东 深圳 电信"},
		{"中国–北京–北京", "联通/北京百度网讯科技有限公司联通节点", "中国 北京 联通"},
		{"中国", "电信/DNS派公共DNS", "中国 电信"},
		{"美国", "Charter_Communications", "美国 Charter Communications"},
		{"美国–佛罗里达州–迈阿密－戴德县–迈阿密", "Comcast有线通信公司", "美国 佛罗里达州 迈阿密－戴德县 迈阿密 Comcast有线通信公司"},
		{"中国–香港–九龙半岛–油尖旺区-尖沙咀", "", "中国 香港 九龙半岛 油尖旺区-尖沙咀"},
		{"IANA", "运营商级NAT-IP地址(Shared_Address_Space)", "运营商级NAT-IP地址"},
		{"IANA", "保留地址(Documentation(TEST-NET-1))", "保留地址"},
		{"IANA", "保留地址(NAT64/DNS64_Discovery)", "保留地址"},
		{"IANA", "Unassigned(未分配)", "Unassigned(未分配)"},
		{"中国–广东–揭阳", "联通(揭东/揭西)", "中国 广东 揭阳 联通(揭东/揭西)"},
		{"中国–广东–清远", "/清远市视讯宽带", "中国 广东 清远 清远市视讯宽带"},
		{"中国–香港", "SoftNET_Telecommunications_(HK)", "中国 香港 SoftNET Telecommunications (HK)"},
		{"中国–香港", "GlobalData将军澳数据中心(CeraNetworks)", "中国 香港 GlobalData将军澳数据中心"},
		{"澳大利亚", " CZ88.NET", "澳大利亚"}, // the pre-2024 placeholder had a leading space
		{"中国–上海", "上海", "中国 上海"},      // the area repeats a part
		{" CZ88.NET", "", ""},           // nothing left
		{"纯真网络", "2026年10月21日IP数据", ""}, // the release stamp
		{"广东省深圳市", "电信", "广东省深圳市 电信"},   // pre-2024 files have no separators
		{"北美地区", "VPN_by_Google_One", "北美地区 VPN by Google One"},
	}
	for _, c := range cases {
		if got := geoip.FormatLocation(c.country, c.area); got != c.want {
			t.Errorf("FormatLocation(%q, %q) = %q, want %q", c.country, c.area, got, c.want)
		}
	}
}

// TestRealDatabase checks a real qqwry.dat when GGBOARD_QQWRY_DAT points to one (never committed; download it
// into a scratch directory). It prints a few lookups for review.
func TestRealDatabase(t *testing.T) {
	path := os.Getenv("GGBOARD_QQWRY_DAT")
	if path == "" {
		t.Skip("GGBOARD_QQWRY_DAT is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := geoip.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("records %d, version %s, size %d", db.Count(), db.Version(), db.Size())
	if db.Count() < 1_000_000 || len(db.Version()) != 8 {
		t.Fatalf("records %d version %q", db.Count(), db.Version())
	}
	for _, ip := range []string{"114.114.114.114", "223.5.5.5", "8.8.8.8", "1.1.1.1", "192.168.1.1", "127.0.0.1",
		"10.0.0.1", "100.64.0.1", "119.29.29.29", "180.76.76.76", "61.135.169.121", "36.152.44.95",
		"202.96.128.86", "140.82.112.3", "103.235.46.39", "255.255.255.255"} {
		c, a, _ := db.Lookup(netip.MustParseAddr(ip))
		t.Logf("%-16s %q  ← %q + %q", ip, db.Locate(netip.MustParseAddr(ip)), c, a)
	}
	if db.Locate(netip.MustParseAddr("114.114.114.114")) == "" {
		t.Fatal("114.114.114.114 has no location")
	}
}
