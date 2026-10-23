//go:build !unix

package main

import "time"

// selfCPU is not available on this platform.
func selfCPU() (time.Duration, bool) { return 0, false }
