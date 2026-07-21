// Command fixture is a minimal newline-delimited JSON-RPC echo server used
// only by internal/stdioupstream's tests as a stand-in child process.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(req.ID),
			"result":  map[string]string{"content": "fixture response for " + req.Method},
		}
		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
	}
}
