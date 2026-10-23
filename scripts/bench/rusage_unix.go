//go:build unix

package main

import (
	"syscall"
	"time"
)

// selfCPU returns the CPU time (user + system) this process has used so far.
func selfCPU() (time.Duration, bool) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, false
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano()), true
}
