// Package anomaly implements Outpost's Tier-2 statistical anomaly
// detection: streaming, deterministic outlier flagging on per-tool
// tools/call latency and error rate. No machine learning, no LLM — just
// Welford's online algorithm for running mean/variance. Detection only:
// nothing here ever blocks a call (ADR-0003 — T2 is fail-open, with no
// per-tool fail-closed override in v1).
package anomaly

import (
	"math"
	"sync"

	"github.com/korneza/outpost/internal/boundedset"
)

const (
	minSamplesForDetection = 20
	stdDevThreshold        = 3.0

	// maxTrackedStats bounds how many distinct (upstream, tool, metric)
	// stat entries the detector keeps at once. tool is client-supplied
	// with no validation of its own by this package, so without a cap
	// an attacker sending a fresh fabricated tool name on every request
	// could grow d.stats without bound (Claude Security finding F10) —
	// two entries per distinct tool name (latency_ms, error_rate).
	// Generous headroom for any real deployment's actual tool count,
	// not a tight operational budget.
	maxTrackedStats = 20_000
)

// Anomaly reports a single call whose latency or error outcome deviated
// from that tool's established baseline.
type Anomaly struct {
	Upstream string
	ToolName string
	Metric   string // "latency_ms" | "error_rate"
	Value    float64
	Mean     float64
	StdDev   float64
}

type runningStat struct {
	count int64
	mean  float64
	m2    float64
}

func (s *runningStat) stddev() float64 {
	if s.count < 2 {
		return 0
	}
	return math.Sqrt(s.m2 / float64(s.count-1))
}

// update folds x into the running statistics (Welford's online algorithm).
func (s *runningStat) update(x float64) {
	s.count++
	delta := x - s.mean
	s.mean += delta / float64(s.count)
	delta2 := x - s.mean
	s.m2 += delta * delta2
}

// Detector tracks per-(upstream, tool, metric) running statistics. A
// Detector is safe for concurrent use.
type Detector struct {
	mu      sync.Mutex
	stats   map[string]*runningStat
	tracked *boundedset.Tracker
}

// New returns an empty Detector.
func New() *Detector {
	return &Detector{stats: make(map[string]*runningStat), tracked: boundedset.New(maxTrackedStats)}
}

func statKey(upstream, tool, metric string) string {
	return upstream + "|" + tool + "|" + metric
}

// Observe records one tools/call's outcome and returns any anomalies
// found — comparing the new sample against history collected *before*
// this call, then folding it into that history either way.
func (d *Detector) Observe(upstream, tool string, latencyMs float64, isError bool) []Anomaly {
	var anomalies []Anomaly
	if a := d.observeMetric(upstream, tool, "latency_ms", latencyMs); a != nil {
		anomalies = append(anomalies, *a)
	}
	errValue := 0.0
	if isError {
		errValue = 1.0
	}
	if a := d.observeMetric(upstream, tool, "error_rate", errValue); a != nil {
		anomalies = append(anomalies, *a)
	}
	return anomalies
}

func (d *Detector) observeMetric(upstream, tool, metric string, value float64) *Anomaly {
	d.mu.Lock()
	defer d.mu.Unlock()

	k := statKey(upstream, tool, metric)
	s, ok := d.stats[k]
	if !ok {
		s = &runningStat{}
		d.stats[k] = s
		if evict, evicted := d.tracked.Add(k); evicted {
			delete(d.stats, evict)
		}
	}

	var anomaly *Anomaly
	if s.count >= minSamplesForDetection {
		mean, sd := s.mean, s.stddev()
		isOutlier := false
		if sd > 0 {
			isOutlier = math.Abs(value-mean) > stdDevThreshold*sd
		} else {
			isOutlier = value != mean
		}
		if isOutlier {
			anomaly = &Anomaly{Upstream: upstream, ToolName: tool, Metric: metric, Value: value, Mean: mean, StdDev: sd}
		}
	}

	s.update(value)
	return anomaly
}
