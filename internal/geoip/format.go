package geoip

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Placeholders of the 纯真 database that carry no location.
const (
	placeholderCountry = "纯真网络" // the release stamp record (255.255.255.0/24)
	placeholderArea    = "CZ88.NET"
)

// placeholderParts are country parts that are dropped: the registry name of reserved ranges ("IANA" / "保留地址
// (This_network)"), the website placeholder and the generic unknowns.
var placeholderParts = []string{"IANA", placeholderArea, "未知", "未知地区", "未知国家"}

// regionSep separates the parts of a country string in the czdb-converted files ("中国–广东–深圳", U+2013). The
// hyphens inside place names ("迈阿密－戴德县", "达拉斯-沃斯堡都会区") are kept.
const regionSep = "–"

// FormatLocation turns a raw record into one human string, parts separated by single spaces:
//
//	"中国–广东–深圳" + "电信"                       → "中国 广东 深圳 电信"
//	"中国–北京–北京" + "联通/北京百度网讯科技有限公司联通节点" → "中国 北京 联通"
//	"美国" + "Charter_Communications"           → "美国 Charter Communications"
//	"IANA" + "局域网IP(Private-Use)"             → "局域网IP"
//	"IANA" + "本机地址/环回地址(Loopback)"          → "本机地址"
//	"澳大利亚" + "CZ88.NET"                       → "澳大利亚"
//	"纯真网络" + "2026年10月21日IP数据"              → ""
//
// Rules: the release stamp record yields ""; the country is split on "–", placeholder and repeated parts are
// dropped; the area (the network: carrier, company or data centre) loses the "CZ88.NET" placeholder, is cut at the
// first "/" outside parentheses when the text before it is a Chinese carrier name ("电信/…", "联通/…", "教育网/…":
// the carrier comes first, the site detail after it; "APNIC/CloudFlare…" is kept), loses a trailing parenthesised
// ASCII translation after Chinese text ("(Private-Use)"), gets "_" replaced by spaces and is dropped when it
// repeats a country part.
func FormatLocation(country, area string) string {
	country, area = strings.TrimSpace(country), strings.TrimSpace(area)
	if country == placeholderCountry {
		return ""
	}
	parts := make([]string, 0, 6)
	add := func(p string) {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" && !slices.Contains(placeholderParts, p) && !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	for p := range strings.SplitSeq(country, regionSep) {
		add(p)
	}
	add(network(area))
	return strings.Join(parts, " ")
}

// network cleans the area string of a record (see FormatLocation).
func network(area string) string {
	if strings.Contains(strings.ToUpper(area), placeholderArea) {
		return ""
	}
	area = strings.TrimLeft(area, "/ ")
	if i := slashOutsideParens(area); i > 0 && !hasASCII(area[:i]) {
		area = area[:i] // "电信/DNS派公共DNS" → "电信"; "APNIC/CloudFlare公共DNS服务器" is kept whole
	}
	area = trimASCIIParen(area)
	return strings.ReplaceAll(area, "_", " ")
}

// slashOutsideParens returns the index of the first "/" outside parentheses, or -1.
func slashOutsideParens(s string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			depth--
		case '/':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// trimASCIIParen drops a trailing "(…)" made of ASCII only when the text before it contains non-ASCII characters:
// "局域网IP(Private-Use)" → "局域网IP", "保留地址(Documentation(TEST-NET-1))" → "保留地址"; "SoftNET_(HK)" stays.
func trimASCIIParen(s string) string {
	if !strings.HasSuffix(s, ")") {
		return s
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				head, group := s[:i], s[i:]
				if isASCII(group) && !isASCII(head) {
					return strings.TrimSpace(head)
				}
				return s
			}
		}
	}
	return s
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// hasASCII reports whether s contains any ASCII character (a carrier prefix such as "电信" has none).
func hasASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < utf8.RuneSelf {
			return true
		}
	}
	return false
}
