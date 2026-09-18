// Platform-independent shapes for the system statistics AI.md PART 21's
// include_system metrics need. Each supported platform supplies its own reader;
// unsupported platforms return errStatsUnsupported and the collector skips the
// affected metric family rather than reporting a wrong value.
package metric

import "errors"

// errStatsUnsupported reports that the running platform has no pure-Go source
// for a statistic. CGO_ENABLED=0 rules out the native APIs some platforms need.
var errStatsUnsupported = errors.New("system statistics unsupported on this platform")

// cpuSample is one cumulative CPU-time reading. Busy percentage is derived from
// the delta between two samples, never from a single one.
type cpuSample struct {
	total uint64
	idle  uint64
}

// memStat is a point-in-time system memory reading.
type memStat struct {
	total       uint64
	used        uint64
	usedPercent float64
}

// diskStat is a point-in-time filesystem reading for one path.
type diskStat struct {
	total       uint64
	used        uint64
	usedPercent float64
}
