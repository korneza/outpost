package reporter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/report"
)

func TestReportPinPostsToControlPlane(t *testing.T) {
	var mu sync.Mutex
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	r := New(srv.URL, 10)
	r.ReportPin(report.PinEvent{Upstream: "up1", ToolName: "echo", SchemaHash: "abc"})
	r.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected control plane to receive the pin event")
	}
	var got report.PinEvent
	if err := json.Unmarshal(received, &got); err != nil || got.ToolName != "echo" {
		t.Fatalf("unexpected payload: %s (err %v)", received, err)
	}
}

func TestReportPinNeverBlocksWhenControlPlaneIsDown(t *testing.T) {
	r := New("http://127.0.0.1:1", 10) // closed port: nothing listens
	done := make(chan struct{})
	go func() {
		r.ReportPin(report.PinEvent{Upstream: "up1", ToolName: "echo"})
		r.Flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ReportPin+Flush must not hang when the control plane is unreachable")
	}
}

func TestBufferDropsOldestWhenFull(t *testing.T) {
	r := New("http://127.0.0.1:1", 2)
	for i := 0; i < 5; i++ {
		r.ReportPin(report.PinEvent{ToolName: "t"})
	}
	if got := r.BufferedCount(); got > 2 {
		t.Fatalf("buffer should be capped at 2, got %d", got)
	}
}

func TestReporterBufferStaysBoundedUnderSustainedConcurrentLoad(t *testing.T) {
	const bufCap = 16
	r := New("http://127.0.0.1:1", bufCap) // closed port: every send fails

	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				r.ReportPin(report.PinEvent{ToolName: "t"})
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("4000 concurrent ReportPin calls must not hang or deadlock")
	}

	r.Flush()
	if got := r.BufferedCount(); got > bufCap {
		t.Fatalf("buffer exceeded its cap of %d under sustained load: got %d", bufCap, got)
	}
}
