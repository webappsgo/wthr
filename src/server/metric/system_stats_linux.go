//go:build linux

// Linux CPU and memory statistics read from procfs. Pure Go, no CGO, per the
// CGO_ENABLED=0 requirement in AI.md PART 7.
package metric

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readCPUSample returns the aggregate CPU time counters from /proc/stat.
func readCPUSample() (cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}

		var sample cpuSample
		for i, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuSample{}, err
			}
			sample.total += v

			// Fields 3 and 4 after "cpu" are idle and iowait; both count as idle.
			if i == 3 || i == 4 {
				sample.idle += v
			}
		}

		return sample, nil
	}

	if err := scanner.Err(); err != nil {
		return cpuSample{}, err
	}

	return cpuSample{}, errStatsUnsupported
}

// readSystemMemory returns total and used memory from /proc/meminfo. Used is
// derived from MemAvailable when the kernel provides it, which accounts for
// reclaimable cache the way the kernel itself does.
func readSystemMemory() (memStat, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return memStat{}, err
	}
	defer f.Close()

	var total, available, free uint64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}

		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}

		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal":
			total = kb * 1024
		case "MemAvailable":
			available = kb * 1024
		case "MemFree":
			free = kb * 1024
		}
	}

	if err := scanner.Err(); err != nil {
		return memStat{}, err
	}

	if total == 0 {
		return memStat{}, errStatsUnsupported
	}

	unused := available
	if unused == 0 {
		unused = free
	}
	if unused > total {
		unused = total
	}

	stat := memStat{total: total, used: total - unused}
	stat.usedPercent = float64(stat.used) / float64(total) * 100

	return stat, nil
}
