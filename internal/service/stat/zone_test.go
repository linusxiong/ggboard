package statsvc_test

import (
	"testing"
	"time"
	_ "time/tzdata" // LoadLocation on hosts without a zone database (the binary embeds it as well)

	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
)

func TestZoneOf(t *testing.T) {
	load := func(name string) *time.Location {
		t.Helper()
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	}
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	type tz = statsvc.TimeZone
	cases := []struct {
		loc  *time.Location
		at   time.Time
		want tz
	}{
		// TZ=<IANA name> (the Docker image): the name, and the offset at that instant (DST included).
		{load("Asia/Shanghai"), summer, tz{Name: "Asia/Shanghai", Offset: 8 * 3600}},
		{load("America/New_York"), summer, tz{Name: "America/New_York", Offset: -4 * 3600}},
		{load("America/New_York"), winter, tz{Name: "America/New_York", Offset: -5 * 3600}},
		{time.UTC, summer, tz{Name: "UTC", Offset: 0}},
		// No TZ: Go loads /etc/localtime and names the zone "Local", which says nothing to a viewer.
		{time.FixedZone("Local", 8*3600), summer, tz{Name: "UTC+08:00", Offset: 8 * 3600}},
		{time.FixedZone("Local", -(3*3600 + 30*60)), summer, tz{Name: "UTC-03:30", Offset: -(3*3600 + 30*60)}},
		{time.FixedZone("Local", 0), summer, tz{Name: "UTC", Offset: 0}},
		{time.FixedZone("", 5*3600+45*60), summer, tz{Name: "UTC+05:45", Offset: 5*3600 + 45*60}},
		// TZ=<path>: the zone name after zoneinfo/, else the offset.
		{time.FixedZone("/usr/share/zoneinfo/Asia/Kolkata", 5*3600+30*60), summer, tz{Name: "Asia/Kolkata", Offset: 5*3600 + 30*60}},
		{time.FixedZone("/etc/zone-custom", 3600), summer, tz{Name: "UTC+01:00", Offset: 3600}},
	}
	for _, c := range cases {
		if got := statsvc.ZoneOf(c.at.In(c.loc)); got != c.want {
			t.Errorf("ZoneOf(%s at %s) = %+v, want %+v", c.loc, c.at.Format(time.DateOnly), got, c.want)
		}
	}
}
