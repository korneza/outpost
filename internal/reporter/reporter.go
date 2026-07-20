// Package reporter sends pin/drift events from the binary to the hosted
// control plane. It is fail-silent by design: a down or slow control
// plane must never affect MCP traffic (ADR-0003's fail-open discipline
// applied to this boundary too). Events queue in a small bounded buffer;
// when full, the oldest queued event is dropped rather than blocking the
// caller or growing unbounded.
package reporter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/korneza/outpost/internal/report"
)

type Reporter struct {
	url    string
	client *http.Client
	mu     sync.Mutex
	queue  [][]byte
	cap    int
}

func New(controlPlaneURL string, bufferCap int) *Reporter {
	return &Reporter{
		url:    controlPlaneURL,
		client: &http.Client{Timeout: 2 * time.Second},
		cap:    bufferCap,
	}
}

func (r *Reporter) ReportPin(ev report.PinEvent) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	r.enqueue("/v1/ingest/pins", body)
}

func (r *Reporter) ReportDrift(ev report.DriftEvent) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	r.enqueue("/v1/ingest/drift", body)
}

func (r *Reporter) enqueue(path string, body []byte) {
	r.mu.Lock()
	if len(r.queue) >= r.cap && r.cap > 0 {
		r.queue = r.queue[1:]
	}
	r.queue = append(r.queue, append([]byte(path+"\x00"), body...))
	r.mu.Unlock()
	go r.drain()
}

// BufferedCount reports how many events are currently queued (test/ops
// visibility only).
func (r *Reporter) BufferedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queue)
}

// Flush blocks until the current queue has been drained (best-effort —
// failed sends are dropped, not retried, matching fail-silent). Test-only
// convenience; production callers don't need to call this.
func (r *Reporter) Flush() {
	r.drain()
}

func (r *Reporter) drain() {
	r.mu.Lock()
	items := r.queue
	r.queue = nil
	r.mu.Unlock()

	for _, item := range items {
		idx := bytes.IndexByte(item, 0)
		if idx < 0 {
			continue
		}
		path, body := string(item[:idx]), item[idx+1:]
		resp, err := r.client.Post(r.url+path, "application/json", bytes.NewReader(body))
		if err != nil {
			continue
		}
		resp.Body.Close()
	}
}
