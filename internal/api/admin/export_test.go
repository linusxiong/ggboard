package admin

import (
	"testing"
	"time"
)

// SetGeoIPUpdateWait shortens how long POST /admin/geoip/update waits before answering 202.
func SetGeoIPUpdateWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := geoipUpdateWait
	geoipUpdateWait = d
	t.Cleanup(func() { geoipUpdateWait = old })
}
