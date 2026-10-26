package geoip

import (
	"testing"
	"time"
)

// SetLimits lowers the download checks for the synthetic test files and restores them after t.
func SetLimits(t *testing.T, records int, maxBytes int64, delay time.Duration) {
	t.Helper()
	r, m, d := minRecords, maxDownload, bootstrapDelay
	minRecords, maxDownload, bootstrapDelay = records, maxBytes, delay
	t.Cleanup(func() { minRecords, maxDownload, bootstrapDelay = r, m, d })
}

// Boot runs the start hook body.
func (s *Service) Boot() { s.boot() }
