package subscription

import (
	"strings"
	"unicode/utf8"
)

// Region detection for node operations (docs/SUBSCRIPTION.md "Node operations"). A node's region is, in order:
//
//  1. the first flag emoji in its name (a pair of regional indicator symbols, e.g. 🇭🇰 → HK);
//  2. the first region of regionTable whose name keyword occurs in the name or a tag (case-insensitive
//     substring; Chinese, English and city names);
//  3. the first region of regionTable whose code token occurs in the name or a tag as a whole word: upper-case
//     ASCII, case-sensitive, not preceded or followed by an ASCII letter ("HK01" and "JP-2" match, "PHK" does
//     not; "CN2" is never China).
//
// The table order is a priority: relay names usually start with the domestic entry city ("上海-香港 IPLC"),
// so CN comes last, and Indonesia (印度尼西亚) comes before India (印度).

// Region is one row of the keyword table.
type Region struct {
	Code     string   // ISO 3166-1 alpha-2
	Names    []string // case-insensitive substrings
	Tokens   []string // case-sensitive whole-word tokens (ISO alpha-2/alpha-3, IATA city codes, operators)
	Excluded []string // the region never matches a name containing one of these (内蒙古 is not Mongolia)
}

// regionTable is the keyword table, in priority order.
var regionTable = []Region{
	{Code: "HK", Names: []string{"香港", "Hong Kong", "HongKong", "深港", "沪港", "京港", "广港", "杭港", "莞港"}, Tokens: []string{"HK", "HKG", "HKT", "HKBN", "HGC", "WTT"}},
	{Code: "MO", Names: []string{"澳门", "澳門", "Macao", "Macau"}, Tokens: []string{"MO", "MAC", "CTM"}},
	{Code: "TW", Names: []string{"台湾", "臺灣", "台灣", "台北", "臺北", "新北", "台中", "高雄", "彰化", "Taiwan", "Taipei"}, Tokens: []string{"TW", "TWN", "TPE", "CHT", "HINET"}},
	{Code: "JP", Names: []string{"日本", "东京", "東京", "大阪", "埼玉", "名古屋", "Japan", "Tokyo", "Osaka"}, Tokens: []string{"JP", "JPN", "TYO", "NRT", "HND", "KIX", "OSA"}},
	{Code: "KR", Names: []string{"韩国", "韓國", "南韩", "南韓", "首尔", "首爾", "春川", "Korea", "Seoul"}, Tokens: []string{"KR", "KOR", "SEL", "ICN"}},
	{Code: "SG", Names: []string{"新加坡", "狮城", "獅城", "Singapore"}, Tokens: []string{"SG", "SGP", "SIN"}},
	{Code: "US", Names: []string{"美国", "美國", "美西", "美东", "美東", "洛杉矶", "洛杉磯", "圣何塞", "聖何塞", "西雅图", "西雅圖", "芝加哥", "纽约", "紐約", "硅谷", "矽谷", "达拉斯", "達拉斯", "凤凰城", "鳳凰城", "波特兰", "弗里蒙特", "United States", "Los Angeles", "San Jose", "Seattle", "Chicago", "New York", "Dallas", "Silicon Valley"}, Tokens: []string{"US", "USA", "LAX", "SJC", "SEA", "ORD", "JFK", "DFW", "NYC"}},
	{Code: "GB", Names: []string{"英国", "英國", "伦敦", "倫敦", "United Kingdom", "Britain", "England", "London"}, Tokens: []string{"GB", "GBR", "UK", "LHR"}},
	{Code: "DE", Names: []string{"德国", "德國", "法兰克福", "法蘭克福", "柏林", "Germany", "Frankfurt", "Berlin"}, Tokens: []string{"DE", "DEU"}},
	{Code: "FR", Names: []string{"法国", "法國", "巴黎", "France", "Paris"}, Tokens: []string{"FR", "FRA", "CDG"}},
	{Code: "NL", Names: []string{"荷兰", "荷蘭", "阿姆斯特丹", "Netherlands", "Amsterdam"}, Tokens: []string{"NL", "NLD", "AMS"}},
	{Code: "RU", Names: []string{"俄罗斯", "俄羅斯", "莫斯科", "伯力", "圣彼得堡", "Russia", "Moscow"}, Tokens: []string{"RU", "RUS"}},
	{Code: "CA", Names: []string{"加拿大", "多伦多", "多倫多", "温哥华", "溫哥華", "蒙特利尔", "Canada", "Toronto", "Vancouver", "Montreal"}, Tokens: []string{"CA", "CAN", "YVR", "YYZ"}},
	{Code: "AU", Names: []string{"澳大利亚", "澳大利亞", "澳洲", "悉尼", "墨尔本", "墨爾本", "Australia", "Sydney", "Melbourne"}, Tokens: []string{"AU", "AUS", "SYD"}},
	{Code: "ID", Names: []string{"印尼", "印度尼西亚", "印度尼西亞", "雅加达", "雅加達", "Indonesia", "Jakarta"}, Tokens: []string{"ID", "IDN", "CGK"}},
	{Code: "IN", Names: []string{"印度", "孟买", "孟買", "India", "Mumbai"}, Tokens: []string{"IN", "IND", "BOM"}},
	{Code: "TH", Names: []string{"泰国", "泰國", "曼谷", "Thailand", "Bangkok"}, Tokens: []string{"TH", "THA", "BKK"}},
	{Code: "VN", Names: []string{"越南", "胡志明", "河内", "河內", "Vietnam", "Hanoi"}, Tokens: []string{"VN", "VNM", "SGN"}},
	{Code: "MY", Names: []string{"马来西亚", "馬來西亞", "吉隆坡", "Malaysia", "Kuala Lumpur"}, Tokens: []string{"MY", "MYS", "KUL"}},
	{Code: "PH", Names: []string{"菲律宾", "菲律賓", "马尼拉", "馬尼拉", "Philippines", "Manila"}, Tokens: []string{"PH", "PHL", "MNL"}},
	{Code: "TR", Names: []string{"土耳其", "伊斯坦布尔", "Turkey", "Türkiye", "Istanbul"}, Tokens: []string{"TR", "TUR", "IST"}},
	{Code: "AE", Names: []string{"阿联酋", "阿聯酋", "迪拜", "杜拜", "United Arab Emirates", "Dubai"}, Tokens: []string{"AE", "ARE", "UAE", "DXB"}},
	{Code: "IL", Names: []string{"以色列", "Israel"}, Tokens: []string{"IL", "ISR"}},
	{Code: "BR", Names: []string{"巴西", "圣保罗", "聖保羅", "Brazil", "Sao Paulo", "São Paulo"}, Tokens: []string{"BR", "BRA", "GRU"}},
	{Code: "AR", Names: []string{"阿根廷", "Argentina"}, Tokens: []string{"AR", "ARG"}},
	{Code: "MX", Names: []string{"墨西哥", "Mexico"}, Tokens: []string{"MX", "MEX"}},
	{Code: "CL", Names: []string{"智利", "Chile"}, Tokens: []string{"CL", "CHL"}},
	{Code: "ZA", Names: []string{"南非", "约翰内斯堡", "South Africa", "Johannesburg"}, Tokens: []string{"ZA", "ZAF", "JNB"}},
	{Code: "EG", Names: []string{"埃及", "Egypt"}, Tokens: []string{"EG", "EGY"}},
	{Code: "NG", Names: []string{"尼日利亚", "尼日利亞", "Nigeria"}, Tokens: []string{"NG", "NGA"}},
	{Code: "IT", Names: []string{"意大利", "義大利", "米兰", "米蘭", "Italy", "Milan"}, Tokens: []string{"IT", "ITA", "MXP"}},
	{Code: "ES", Names: []string{"西班牙", "马德里", "馬德里", "Spain", "Madrid"}, Tokens: []string{"ES", "ESP", "MAD"}},
	{Code: "PT", Names: []string{"葡萄牙", "里斯本", "Portugal", "Lisbon"}, Tokens: []string{"PT", "PRT"}},
	{Code: "CH", Names: []string{"瑞士", "苏黎世", "蘇黎世", "Switzerland", "Zurich"}, Tokens: []string{"CH", "CHE", "ZRH"}},
	{Code: "SE", Names: []string{"瑞典", "斯德哥尔摩", "Sweden", "Stockholm"}, Tokens: []string{"SE", "SWE"}},
	{Code: "NO", Names: []string{"挪威", "Norway", "Oslo"}, Tokens: []string{"NO", "NOR"}},
	{Code: "FI", Names: []string{"芬兰", "芬蘭", "赫尔辛基", "Finland", "Helsinki"}, Tokens: []string{"FI", "FIN", "HEL"}},
	{Code: "DK", Names: []string{"丹麦", "丹麥", "Denmark", "Copenhagen"}, Tokens: []string{"DK", "DNK"}},
	{Code: "PL", Names: []string{"波兰", "波蘭", "华沙", "華沙", "Poland", "Warsaw"}, Tokens: []string{"PL", "POL", "WAW"}},
	{Code: "AT", Names: []string{"奥地利", "奧地利", "维也纳", "維也納", "Austria", "Vienna"}, Tokens: []string{"AT", "AUT", "VIE"}},
	{Code: "BE", Names: []string{"比利时", "比利時", "Belgium", "Brussels"}, Tokens: []string{"BE", "BEL"}},
	{Code: "IE", Names: []string{"爱尔兰", "愛爾蘭", "都柏林", "Ireland", "Dublin"}, Tokens: []string{"IE", "IRL", "DUB"}},
	{Code: "UA", Names: []string{"乌克兰", "烏克蘭", "Ukraine", "Kyiv"}, Tokens: []string{"UA", "UKR"}},
	{Code: "KZ", Names: []string{"哈萨克斯坦", "哈薩克", "Kazakhstan"}, Tokens: []string{"KZ", "KAZ"}},
	{Code: "MN", Names: []string{"蒙古", "Mongolia"}, Tokens: []string{"MN", "MNG"}, Excluded: []string{"内蒙古", "內蒙古"}},
	{Code: "NZ", Names: []string{"新西兰", "新西蘭", "紐西蘭", "奥克兰", "New Zealand", "Auckland"}, Tokens: []string{"NZ", "NZL", "AKL"}},
	{Code: "CN", Names: []string{"中国", "中國", "回国", "回國", "内蒙古", "內蒙古", "北京", "上海", "广州", "廣州", "深圳", "杭州", "江苏", "江蘇", "China"}, Tokens: []string{"CN", "CHN"}},
}

// regionAliases maps alternative codes to ISO codes (UK is common in node names, the ISO code is GB).
var regionAliases = map[string]string{"UK": "GB"}

// regionIndex maps a region code to its table position (sort order).
var regionIndex = func() map[string]int {
	m := make(map[string]int, len(regionTable))
	for i, r := range regionTable {
		m[r.Code] = i
	}
	return m
}()

// lowerNames caches the lower-cased name keywords of regionTable.
var lowerNames = func() [][]string {
	out := make([][]string, len(regionTable))
	for i, r := range regionTable {
		for _, n := range r.Names {
			out[i] = append(out[i], strings.ToLower(n))
		}
	}
	return out
}()

// Regions returns the region keyword table (read-only) in priority order.
func Regions() []Region { return regionTable }

// RegionCode canonicalizes a user-supplied region code (upper case, UK → GB). ok is false for codes outside
// the keyword table.
func RegionCode(s string) (code string, ok bool) {
	code = strings.ToUpper(strings.TrimSpace(s))
	if a, alias := regionAliases[code]; alias {
		code = a
	}
	_, ok = regionIndex[code]
	return code, ok
}

// RegionFlag returns the flag emoji of an ISO 3166-1 alpha-2 code ("" when code is not two letters A-Z).
func RegionFlag(code string) string {
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return ""
	}
	return string([]rune{0x1F1E6 + rune(code[0]-'A'), 0x1F1E6 + rune(code[1]-'A')})
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// flagRegion returns the code of the first flag emoji in s (any two regional indicators, also outside the
// table) and the flag itself.
func flagRegion(s string) (code, flag string) {
	var prev rune
	for i, r := range s {
		if isRegionalIndicator(r) {
			if prev != 0 {
				code = string([]byte{byte('A' + prev - 0x1F1E6), byte('A' + r - 0x1F1E6)})
				return code, s[i-utf8.RuneLen(prev) : i+utf8.RuneLen(r)]
			}
			prev = r
			continue
		}
		prev = 0
	}
	return "", ""
}

// HasFlag reports whether s contains a flag emoji.
func HasFlag(s string) bool {
	code, _ := flagRegion(s)
	return code != ""
}

func excluded(r *Region, s string) bool {
	for _, x := range r.Excluded {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// hasToken reports whether tok occurs in s with no ASCII letter directly before or after it ("CN" in "CN2"
// does not count: CN2 is a carrier route, not a region).
func hasToken(s, tok string) bool {
	for off := 0; ; {
		i := strings.Index(s[off:], tok)
		if i < 0 {
			return false
		}
		start, end := off+i, off+i+len(tok)
		before := start == 0 || !isASCIILetter(s[start-1])
		after := end == len(s) || (!isASCIILetter(s[end]) && !(tok == "CN" && s[end] == '2'))
		if before && after {
			return true
		}
		off = start + 1
	}
}

func isASCIILetter(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }

// DetectRegion returns the ISO code of the region of a node named name with tags ("" when unknown); see the
// rules at the top of this file.
func DetectRegion(name string, tags []string) string {
	if code, _ := flagRegion(name); code != "" {
		if a, ok := regionAliases[code]; ok {
			return a
		}
		return code
	}
	texts := append([]string{name}, tags...)
	lower := make([]string, len(texts))
	for i, t := range texts {
		lower[i] = strings.ToLower(t)
	}
	for i := range regionTable {
		r := &regionTable[i]
		for _, kw := range lowerNames[i] {
			for j, t := range lower {
				if strings.Contains(t, kw) && !excluded(r, texts[j]) {
					return r.Code
				}
			}
		}
	}
	for i := range regionTable {
		r := &regionTable[i]
		for _, tok := range r.Tokens {
			for _, t := range texts {
				if hasToken(t, tok) && !excluded(r, t) {
					return r.Code
				}
			}
		}
	}
	return ""
}
