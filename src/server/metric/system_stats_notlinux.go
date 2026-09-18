//go:build !linux

// CPU and memory statistics have no pure-Go source outside Linux, so those two
// metric families are simply absent there. AI.md PART 21 treats system metrics
// as optional, and reporting nothing is preferable to reporting a guess.
package metric

// readCPUSample reports that CPU sampling is unavailable on this platform.
func readCPUSample() (cpuSample, error) {
	return cpuSample{}, errStatsUnsupported
}

// readSystemMemory reports that memory statistics are unavailable on this platform.
func readSystemMemory() (memStat, error) {
	return memStat{}, errStatsUnsupported
}
