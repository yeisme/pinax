//go:build !windows

package app

import "syscall"

func captureMonitorOSResource(snap *monitorResourceSnapshot) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return
	}
	snap.cpuSupported = true
	snap.cpuUserMicros = timevalMicros(usage.Utime)
	snap.cpuSystemMicros = timevalMicros(usage.Stime)
	if usage.Maxrss > 0 {
		snap.peakRSSBytes = uint64(usage.Maxrss) * 1024
		snap.rssSupported = true
	}
}

func timevalMicros(tv syscall.Timeval) int64 {
	return int64(tv.Sec)*1_000_000 + int64(tv.Usec)
}
