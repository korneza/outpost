// Command fixture is a minimal newline-delimited JSON-RPC echo server used
// only by internal/stdioupstream's tests as a stand-in child process.
//
// "-bigline=<n>" makes every response include an n-byte padding field —
// used to prove Caller.Call can read a single response line larger than
// bufio.Scanner's 64KB default token size, a real size a legitimate
// tools/call result (e.g. one embedding a modest file) can plausibly
// reach.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
}

func main() {
	bigLine := flag.Int("bigline", 0, "pad every response with this many extra bytes")
	flag.Parse()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		if len(req.ID) == 0 {
			// JSON-RPC 2.0: a message with no id is a notification and
			// MUST NOT receive a response. A spec-compliant test fixture
			// needs to actually behave this way — real MCP servers do,
			// and stdioupstream.Caller.Call relies on that being true.
			continue
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			// env_secret lets a test observe exactly what this child's
			// environment actually contains, without any additional
			// fixture mode — it's simply the value (or "" if unset) of
			// whatever env var name the test cares about checking.
			"result": map[string]string{
				"content":    "fixture response for " + req.Method,
				"env_secret": os.Getenv("OUTPOST_TEST_SECRET"),
				"padding":    strings.Repeat("a", *bigLine),
			},
		}
		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
	}
}
