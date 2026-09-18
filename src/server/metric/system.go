// System and Go runtime metrics per AI.md PART 21.
// System metrics are gated by server.metrics.include_system and runtime
// metrics by server.metrics.include_runtime; both default to true.
package metric

import (
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// System metrics (include_system)
	SystemCPUUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_cpu_usage_percent",
			Help: "Current CPU usage percentage",
		},
	)

	SystemMemoryUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_memory_usage_percent",
			Help: "Current memory usage percentage",
		},
	)

	SystemMemoryUsed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_memory_used_bytes",
			Help: "Memory used in bytes",
		},
	)

	SystemMemoryTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_system_memory_total_bytes",
			Help: "Total memory in bytes",
		},
	)

	SystemDiskUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_system_disk_usage_percent",
			Help: "Disk usage percentage",
		},
		[]string{"path"},
	)

	SystemDiskUsed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_system_disk_used_bytes",
			Help: "Disk used in bytes",
		},
		[]string{"path"},
	)

	SystemDiskTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wthr_system_disk_total_bytes",
			Help: "Total disk in bytes",
		},
		[]string{"path"},
	)

	// Go runtime metrics (include_runtime)
	GoGoroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_go_goroutines",
			Help: "Number of goroutines",
		},
	)

	GoMemAlloc = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_go_mem_alloc_bytes",
			Help: "Bytes allocated and in use",
		},
	)

	GoMemSys = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wthr_go_mem_sys_bytes",
			Help: "Bytes obtained from system",
		},
	)

	GoGCRuns = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "wthr_go_gc_runs_total",
			Help: "Total number of GC runs",
		},
	)

	GoGCPauseTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "wthr_go_gc_pause_total_seconds",
			Help: "Total GC pause time in seconds",
		},
	)
)

// DefaultCollectInterval is how often system and runtime metrics are refreshed.
const DefaultCollectInterval = 15 * time.Second

// SystemCollector periodically refreshes system and Go runtime metrics.
type SystemCollector struct {
	dataDir        string
	interval       time.Duration
	includeSystem  bool
	includeRuntime bool

	stop     chan struct{}
	stopOnce sync.Once

	lastGC      uint32
	lastPauseNs uint64
	lastCPU     cpuSample
	hasCPU      bool
}

// NewSystemCollector creates a collector. An empty dataDir disables disk
// metrics; platforms without a supported stats source simply report nothing for
// the metrics they cannot read.
func NewSystemCollector(dataDir string, interval time.Duration, includeSystem, includeRuntime bool) *SystemCollector {
	if interval <= 0 {
		interval = DefaultCollectInterval
	}
	return &SystemCollector{
		dataDir:        dataDir,
		interval:       interval,
		includeSystem:  includeSystem,
		includeRuntime: includeRuntime,
		stop:           make(chan struct{}),
	}
}

// StartSystemCollector begins collection in the background.
func (c *SystemCollector) StartSystemCollector() {
	go c.run()
}

// StopSystemCollector halts collection. Safe to call more than once.
func (c *SystemCollector) StopSystemCollector() {
	c.stopOnce.Do(func() { close(c.stop) })
}

func (c *SystemCollector) run() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.CollectOnce()
	for {
		select {
		case <-ticker.C:
			c.CollectOnce()
		case <-c.stop:
			return
		}
	}
}

// CollectOnce performs a single collection pass.
func (c *SystemCollector) CollectOnce() {
	if c.includeSystem {
		c.collectCPU()
		c.collectMemory()
		c.collectDisk()
	}
	if c.includeRuntime {
		c.collectRuntime()
	}
}

// collectCPU derives busy percentage from the delta between two samples, so the
// first pass after start only records a baseline.
func (c *SystemCollector) collectCPU() {
	sample, err := readCPUSample()
	if err != nil {
		return
	}

	if c.hasCPU {
		totalDelta := float64(sample.total - c.lastCPU.total)
		idleDelta := float64(sample.idle - c.lastCPU.idle)
		if totalDelta > 0 {
			busy := (totalDelta - idleDelta) / totalDelta * 100
			if busy < 0 {
				busy = 0
			}
			if busy > 100 {
				busy = 100
			}
			SystemCPUUsage.Set(busy)
		}
	}

	c.lastCPU = sample
	c.hasCPU = true
}

func (c *SystemCollector) collectMemory() {
	stat, err := readSystemMemory()
	if err != nil {
		return
	}
	SystemMemoryUsage.Set(stat.usedPercent)
	SystemMemoryUsed.Set(float64(stat.used))
	SystemMemoryTotal.Set(float64(stat.total))
}

func (c *SystemCollector) collectDisk() {
	if c.dataDir == "" {
		return
	}
	stat, err := readDiskUsage(c.dataDir)
	if err != nil {
		return
	}
	SystemDiskUsage.WithLabelValues(c.dataDir).Set(stat.usedPercent)
	SystemDiskUsed.WithLabelValues(c.dataDir).Set(float64(stat.used))
	SystemDiskTotal.WithLabelValues(c.dataDir).Set(float64(stat.total))
}

// collectRuntime advances the GC counters by their delta since the previous
// pass; MemStats reports cumulative totals and a Prometheus counter is
// monotonic, so adding the raw totals every pass would multiply-count them.
func (c *SystemCollector) collectRuntime() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	GoGoroutines.Set(float64(runtime.NumGoroutine()))
	GoMemAlloc.Set(float64(m.Alloc))
	GoMemSys.Set(float64(m.Sys))

	if m.NumGC > c.lastGC {
		GoGCRuns.Add(float64(m.NumGC - c.lastGC))
		c.lastGC = m.NumGC
	}
	if m.PauseTotalNs > c.lastPauseNs {
		GoGCPauseTotal.Add(float64(m.PauseTotalNs-c.lastPauseNs) / 1e9)
		c.lastPauseNs = m.PauseTotalNs
	}
}
