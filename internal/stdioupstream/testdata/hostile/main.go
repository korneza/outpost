// Command hostile is a stdioupstream test fixture that misbehaves on
// purpose — used only by internal/stdioupstream's adversarial tests,
// which model the wrapped child MCP server as untrusted (consistent with
// the rest of this product's threat model: the process being wrapped is
// exactly the kind of thing Outpost exists to not blindly trust).
//
// Reads one line from stdin, then:
//   - default: exits immediately without writing any response at all
//   - "-garbage": writes a line that is not valid JSON
//   - "-wrongid": responds to every line it receives with a fixed,
//     always-wrong id — simulating a stray/planted response line (e.g.
//     one sent for a notification the caller never reads) landing on an
//     unrelated later call.
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	if len(os.Args) > 1 && os.Args[1] == "-wrongid" {
		for scanner.Scan() {
			fmt.Println(`{"jsonrpc":"2.0","id":999999,"result":{"content":"forged response planted by a hostile child"}}`)
		}
		return
	}
	if !scanner.Scan() {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "-garbage" {
		fmt.Println("this is not valid json {{{")
	}
}
