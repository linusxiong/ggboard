// Package geoiptest builds small synthetic qqwry.dat files for tests (no real database is ever committed).
package geoiptest

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Range is one IP range of a synthetic database. Country uses "–" between parts like the real files
// ("中国–广东–深圳"); both strings are stored in GBK.
type Range struct {
	Start, End string // IPv4, inclusive
	Country    string
	Area       string
}

// Sample is a small database covering the addresses the tests use. It exercises every record layout: plain
// strings, a mode-1 redirect (a repeated country + area), a mode-1 redirect to a mode-2 record, mode-2 redirects
// with a plain or redirected area, an area redirect, and a gap between ranges (198.51.100.0/24 has no range).
var Sample = []Range{
	{Start: "0.0.0.1", End: "0.255.255.255", Country: "IANA", Area: "保留地址(This_network)"},
	{Start: "1.1.1.0", End: "1.1.1.255", Country: "澳大利亚", Area: "APNIC/CloudFlare公共DNS服务器"},
	{Start: "8.8.4.4", End: "8.8.4.4", Country: "美国–加利福尼亚州–圣克拉拉–山景城", Area: "谷歌公司DNS服务器"},
	{Start: "8.8.8.8", End: "8.8.8.8", Country: "美国–加利福尼亚州–圣克拉拉–山景城", Area: "谷歌公司DNS服务器"},
	{Start: "10.0.0.0", End: "10.255.255.255", Country: "IANA", Area: "局域网IP(Private-Use)"},
	{Start: "114.114.114.114", End: "114.114.114.114", Country: "中国–江苏–南京", Area: "南京信风网络科技有限公司GreatbitDNS服务器"},
	{Start: "114.114.114.115", End: "114.114.115.255", Country: "中国–江苏–南京", Area: "电信"},
	{Start: "127.0.0.0", End: "127.255.255.255", Country: "IANA", Area: "本机地址/环回地址(Loopback)"},
	{Start: "192.168.0.0", End: "192.168.255.255", Country: "IANA", Area: "局域网IP(Private-Use)"},
	{Start: "203.0.113.0", End: "203.0.113.127", Country: "中国–广东–深圳", Area: "电信"},
	{Start: "203.0.113.128", End: "203.0.113.255", Country: "中国–广东–深圳", Area: "CZ88.NET"},
	{Start: "223.5.5.5", End: "223.5.5.5", Country: "中国–浙江–杭州", Area: "阿里巴巴anycast公共DNS"},
	{Start: "223.5.5.6", End: "223.5.5.255", Country: "中国–北京–北京", Area: "联通/北京百度网讯科技有限公司联通节点"},
}

// SampleVersion is the release date stamped into Build's output by default.
const SampleVersion = "20261021"

// Build returns a qqwry.dat with ranges (sorted by Start, non-overlapping) and the release stamp record
// "纯真网络" / "<version>IP数据" for 255.255.255.0/24 (none when version is ""). Repeated strings are written once
// and referenced through the format's redirects, like the real file.
func Build(ranges []Range, version string) []byte {
	rs := slices.Clone(ranges)
	if version != "" {
		rs = append(rs, Range{Start: "255.255.255.0", End: "255.255.255.255", Country: "纯真网络",
			Area: fmt.Sprintf("%s年%s月%s日IP数据", version[:4], trimZero(version[4:6]), trimZero(version[6:8]))})
	}
	slices.SortFunc(rs, func(a, b Range) int { return cmp.Compare(ip4(a.Start), ip4(b.Start)) })

	out := make([]byte, 8, 4096)     // header filled in at the end
	countryAt := map[string]uint32{} // country string -> offset of its bytes
	areaAt := map[string]uint32{}    // area string -> offset of its bytes
	pairAt := map[[2]string]uint32{} // country + area -> offset of a location usable by a mode-1 redirect
	type idx struct{ start, off uint32 }
	index := make([]idx, 0, len(rs))
	put3 := func(v uint32) { out = append(out, byte(v), byte(v>>8), byte(v>>16)) }
	cstr := func(s string) uint32 {
		at := uint32(len(out))
		out = append(out, gbk(s)...)
		out = append(out, 0)
		return at
	}
	writeArea := func(a string) {
		if at, ok := areaAt[a]; ok {
			out = append(out, 0x02)
			put3(at)
			return
		}
		areaAt[a] = cstr(a)
	}

	for _, r := range rs {
		rec := uint32(len(out))
		index = append(index, idx{start: ip4(r.Start), off: rec})
		out = binary.LittleEndian.AppendUint32(out, ip4(r.End))
		loc := uint32(len(out))
		pair := [2]string{r.Country, r.Area}
		if at, seen := pairAt[pair]; seen { // mode 1: country and area both at a previous location
			out = append(out, 0x01)
			put3(at)
			continue
		}
		if c, ok := countryAt[r.Country]; ok { // mode 2: shared country, own area
			out = append(out, 0x02)
			put3(c)
		} else {
			countryAt[r.Country] = cstr(r.Country)
		}
		writeArea(r.Area)
		pairAt[pair] = loc
	}

	first := uint32(len(out))
	for _, e := range index {
		out = binary.LittleEndian.AppendUint32(out, e.start)
		put3(e.off)
	}
	last := uint32(len(out)) - 7
	binary.LittleEndian.PutUint32(out[0:4], first)
	binary.LittleEndian.PutUint32(out[4:8], last)
	return out
}

// Install writes Build(ranges, version) as <dataDir>/geoip/qqwry.dat (the file the service loads on first use).
func Install(t testing.TB, dataDir string, ranges []Range, version string) string {
	t.Helper()
	dir := filepath.Join(dataDir, "geoip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "qqwry.dat")
	if err := os.WriteFile(path, Build(ranges, version), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func ip4(s string) uint32 {
	a := netip.MustParseAddr(s).As4()
	return binary.BigEndian.Uint32(a[:])
}

func gbk(s string) []byte {
	b, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		panic(fmt.Sprintf("geoiptest: %q is not representable in GBK: %v", s, err))
	}
	return b
}

func trimZero(s string) string {
	if len(s) == 2 && s[0] == '0' {
		return s[1:]
	}
	return s
}
