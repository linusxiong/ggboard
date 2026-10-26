package geoip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// qqwry.dat layout (all integers little-endian):
//
//	header   first index offset (4) | last index offset (4)
//	records  end IP (4) | location
//	index    start IP (4) | record offset (3), sorted by start IP, 7 bytes per entry
//
// A location is a NUL-terminated country string followed by an area string, or a redirect: 0x01 + offset (country
// and area both at offset, where the country may itself be a 0x02 redirect) or 0x02 + offset (country at offset,
// area right after the 4 redirect bytes). An area may also be 0x01/0x02 + offset (offset 0 = no area). Strings are
// GBK (decoded as GB18030, its superset).

const (
	indexLen   = 7
	headerLen  = 8
	maxStrLen  = 1024 // longer strings are treated as corruption
	modeRedir1 = 0x01
	modeRedir2 = 0x02
)

// ErrFormat reports a file that is not a well-formed qqwry.dat.
var ErrFormat = errors.New("not a valid qqwry.dat")

// DB is a parsed qqwry.dat. It is immutable, so lookups are safe for concurrent use.
type DB struct {
	data    []byte
	first   uint32 // offset of the first index entry
	count   int
	version string
}

// Parse checks data completely (header, sorted index, every record and string in bounds) and returns the
// database. data must not be modified afterwards. Because every record is checked here, lookups never fail on a
// parsed DB.
func Parse(data []byte) (*DB, error) {
	if len(data) < headerLen+indexLen {
		return nil, fmt.Errorf("%w: %d bytes", ErrFormat, len(data))
	}
	first := binary.LittleEndian.Uint32(data[0:4])
	last := binary.LittleEndian.Uint32(data[4:8])
	if first < headerLen || last < first || (last-first)%indexLen != 0 || uint64(last)+indexLen > uint64(len(data)) {
		return nil, fmt.Errorf("%w: bad index bounds %d..%d in %d bytes", ErrFormat, first, last, len(data))
	}
	db := &DB{data: data, first: first, count: int((last-first)/indexLen) + 1}
	var prev uint32
	for i := range db.count {
		start, off := db.entry(i)
		if i > 0 && start <= prev {
			return nil, fmt.Errorf("%w: index entry %d is not sorted", ErrFormat, i)
		}
		prev = start
		if off < headerLen || off >= first {
			return nil, fmt.Errorf("%w: index entry %d points outside the records", ErrFormat, i)
		}
		end, _, _, err := db.record(off)
		if err != nil {
			return nil, fmt.Errorf("%w: record of entry %d: %w", ErrFormat, i, err)
		}
		if end < start {
			return nil, fmt.Errorf("%w: record of entry %d ends before it starts", ErrFormat, i)
		}
	}
	db.version = db.stamp()
	return db, nil
}

// Count returns the number of IP ranges.
func (db *DB) Count() int { return db.count }

// Size returns the size of the file in bytes.
func (db *DB) Size() int { return len(db.data) }

// Version returns the release date the file carries in its last record ("纯真网络" / "2026年10月21日IP数据") as
// YYYYMMDD, or "" when it has none.
func (db *DB) Version() string { return db.version }

// Lookup returns the decoded country and area strings of the range containing ip. ok is false for IPv6
// addresses and addresses outside every range.
func (db *DB) Lookup(ip netip.Addr) (country, area string, ok bool) {
	ip = ip.Unmap()
	if !ip.Is4() {
		return "", "", false
	}
	b := ip.As4()
	v := binary.BigEndian.Uint32(b[:])
	i := sort.Search(db.count, func(i int) bool {
		start, _ := db.entry(i)
		return start > v
	}) - 1
	if i < 0 {
		return "", "", false
	}
	_, off := db.entry(i)
	end, c, a, err := db.record(off)
	if err != nil || v > end {
		return "", "", false
	}
	return decodeGBK(c), decodeGBK(a), true
}

// Locate returns the formatted location of ip (see FormatLocation), "" when unknown.
func (db *DB) Locate(ip netip.Addr) string {
	c, a, ok := db.Lookup(ip)
	if !ok {
		return ""
	}
	return FormatLocation(c, a)
}

// entry returns the start IP and record offset of index entry i (0 <= i < count).
func (db *DB) entry(i int) (start, off uint32) {
	p := int(db.first) + i*indexLen
	return binary.LittleEndian.Uint32(db.data[p : p+4]), u24(db.data[p+4 : p+7])
}

func u24(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 }

var errBounds = errors.New("offset out of bounds")

// at returns the 3-byte offset stored at p.
func (db *DB) at(p uint32) (uint32, error) {
	if uint64(p)+3 > uint64(len(db.data)) {
		return 0, errBounds
	}
	return u24(db.data[p : p+3]), nil
}

// cstr returns the NUL-terminated string at p (without the NUL).
func (db *DB) cstr(p uint32) ([]byte, error) {
	if uint64(p) >= uint64(len(db.data)) {
		return nil, errBounds
	}
	rest := db.data[p:]
	if len(rest) > maxStrLen {
		rest = rest[:maxStrLen]
	}
	n := bytes.IndexByte(rest, 0)
	if n < 0 {
		return nil, errors.New("unterminated string")
	}
	return rest[:n], nil
}

// area reads an area field at p: a string or a redirect to one (offset 0 = empty).
func (db *DB) area(p uint32) ([]byte, error) {
	if uint64(p) >= uint64(len(db.data)) {
		return nil, errBounds
	}
	if m := db.data[p]; m == modeRedir1 || m == modeRedir2 {
		o, err := db.at(p + 1)
		if err != nil || o == 0 {
			return nil, err
		}
		return db.cstr(o)
	}
	return db.cstr(p)
}

// record reads the record at off: its end IP and the raw (GBK) country and area strings.
func (db *DB) record(off uint32) (end uint32, country, area []byte, err error) {
	if uint64(off)+5 > uint64(len(db.data)) {
		return 0, nil, nil, errBounds
	}
	end = binary.LittleEndian.Uint32(db.data[off : off+4])
	p := off + 4
	switch db.data[p] {
	case modeRedir1:
		q, err := db.at(p + 1)
		if err != nil {
			return 0, nil, nil, err
		}
		if uint64(q) >= uint64(len(db.data)) {
			return 0, nil, nil, errBounds
		}
		if db.data[q] == modeRedir2 {
			c, err := db.at(q + 1)
			if err != nil {
				return 0, nil, nil, err
			}
			if country, err = db.cstr(c); err != nil {
				return 0, nil, nil, err
			}
			area, err = db.area(q + 4)
			return end, country, area, err
		}
		if country, err = db.cstr(q); err != nil {
			return 0, nil, nil, err
		}
		area, err = db.area(q + uint32(len(country)) + 1)
		return end, country, area, err
	case modeRedir2:
		c, err := db.at(p + 1)
		if err != nil {
			return 0, nil, nil, err
		}
		if country, err = db.cstr(c); err != nil {
			return 0, nil, nil, err
		}
		area, err = db.area(off + 8)
		return end, country, area, err
	default:
		if country, err = db.cstr(p); err != nil {
			return 0, nil, nil, err
		}
		area, err = db.area(p + uint32(len(country)) + 1)
		return end, country, area, err
	}
}

// stampRe matches the release date in the last record's area ("2026年10月21日IP数据").
var stampRe = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日`)

// stamp reads the release date of the last record, which covers 255.255.255.0/24 and is "纯真网络" /
// "<date>IP数据" in every release of the file.
func (db *DB) stamp() string {
	_, off := db.entry(db.count - 1)
	_, c, a, err := db.record(off)
	if err != nil || decodeGBK(c) != placeholderCountry {
		return ""
	}
	m := stampRe.FindStringSubmatch(decodeGBK(a))
	if m == nil {
		return ""
	}
	pad := func(s string) string {
		if len(s) == 1 {
			return "0" + s
		}
		return s
	}
	return m[1] + pad(m[2]) + pad(m[3])
}

// decodeGBK converts a GBK string to UTF-8 (ASCII is returned as is).
func decodeGBK(b []byte) string {
	ascii := true
	for _, c := range b {
		if c >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b)
	if err != nil {
		return ""
	}
	return string(out)
}
