package tracing

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStartCallSpanRecordsAttributesNoPayload(t *testing.T) {
	var buf bytes.Buffer
	tp, err := NewProvider(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := StartCallSpan(context.Background(), tp, "up1", "tools/call", "echo")
	EndCallSpan(span, 12.5, true)
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{`"upstream"`, `"up1"`, `"tool"`, `"echo"`, `"method"`, `"tools/call"`, `"success"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("exported span missing %q, got: %s", want, out)
		}
	}
	for _, banned := range []string{"arguments", "result", "payload"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Fatalf("exported span must never contain %q, got: %s", banned, out)
		}
	}
}
