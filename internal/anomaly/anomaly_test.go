package anomaly

import (
	"testing"
)

func TestNoAnomalyBelowMinimumSamples(t *testing.T) {
	d := New()
	// Wildly varying latency, but fewer than minSamplesForDetection calls —
	// there isn't enough history yet to call anything "abnormal".
	latencies := []float64{5, 500, 8, 900, 3}
	for _, lat := range latencies {
		if anomalies := d.Observe("files", "files.read", lat, false); len(anomalies) != 0 {
			t.Fatalf("Observe(%v) = %v, want none — too few samples for detection", lat, anomalies)
		}
	}
}

func TestFlagsLatencyOutlierAfterEnoughSamples(t *testing.T) {
	d := New()
	// 25 consistent, low-variance latencies establish a tight baseline.
	for i := 0; i < 25; i++ {
		d.Observe("files", "files.read", 10, false)
	}
	// A wildly higher latency should now stand out.
	anomalies := d.Observe("files", "files.read", 5000, false)
	found := false
	for _, a := range anomalies {
		if a.Metric == "latency_ms" {
			found = true
			if a.Value != 5000 {
				t.Errorf("anomaly.Value = %v, want 5000", a.Value)
			}
		}
	}
	if !found {
		t.Fatalf("anomalies = %v, want a latency_ms anomaly for a 5000ms call against a ~10ms baseline", anomalies)
	}
}

func TestNoLatencyAnomalyForConsistentValues(t *testing.T) {
	d := New()
	for i := 0; i < 30; i++ {
		if anomalies := d.Observe("files", "files.read", 10, false); len(anomalies) != 0 {
			t.Fatalf("Observe(10) = %v, want none — identical values are never anomalous relative to themselves", anomalies)
		}
	}
}

func TestFlagsFirstErrorAfterCleanStreakDespiteZeroVariance(t *testing.T) {
	// The realistic case: a tool has been 100% successful, so error_rate's
	// stddev is exactly 0. A plain "> mean + 3*stddev" check would never
	// fire here — the zero-variance special case must catch it.
	d := New()
	for i := 0; i < 25; i++ {
		d.Observe("files", "files.read", 10, false)
	}
	anomalies := d.Observe("files", "files.read", 10, true)
	found := false
	for _, a := range anomalies {
		if a.Metric == "error_rate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("anomalies = %v, want an error_rate anomaly for the first failure after 25 clean calls", anomalies)
	}
}

func TestMetricsArePerToolAndPerUpstream(t *testing.T) {
	d := New()
	for i := 0; i < 25; i++ {
		d.Observe("files", "files.read", 10, false)
	}
	// A fresh tool/upstream combination has no history yet, so a very
	// different latency on it must not be flagged.
	if anomalies := d.Observe("files", "files.write", 5000, false); len(anomalies) != 0 {
		t.Fatalf("anomalies = %v, want none for a different tool with no history", anomalies)
	}
	if anomalies := d.Observe("other-upstream", "files.read", 5000, false); len(anomalies) != 0 {
		t.Fatalf("anomalies = %v, want none for the same tool name on a different upstream", anomalies)
	}
}

// TestTrackedStatCountIsBounded guards against Claude Security finding
// F10: tool is client-supplied with no cardinality validation upstream
// of Observe, and each distinct (upstream, tool, metric) key creates a
// permanent stats entry with no eviction — an attacker sending a fresh
// fabricated tool name per request could grow d.stats without bound
// (two entries per distinct tool name: latency_ms and error_rate).
func TestTrackedStatCountIsBounded(t *testing.T) {
	d := New()
	for i := 0; i < maxTrackedStats+500; i++ {
		d.Observe("files", "tool-"+string(rune(i)), 10, false)
	}
	d.mu.Lock()
	n := len(d.stats)
	d.mu.Unlock()
	if n > maxTrackedStats {
		t.Fatalf("tracked stat count = %d, want capped at %d", n, maxTrackedStats)
	}
}
