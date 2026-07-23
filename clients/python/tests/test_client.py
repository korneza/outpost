import json
import os
import sys
import threading
import unittest
import warnings
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from outpost_client import OutpostClient  # noqa: E402


def make_server(via):
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            self.rfile.read(length)
            body = json.dumps({"jsonrpc": "2.0", "id": 1, "result": {"via": via}}).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, fmt, *args):
            pass  # silence per-request logging in test output

    server = HTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server


class TestOutpostClient(unittest.TestCase):
    def test_call_uses_proxy_when_reachable(self):
        proxy = make_server("proxy")
        direct = make_server("direct")
        try:
            client = OutpostClient(
                proxy_url=f"http://127.0.0.1:{proxy.server_port}",
                direct_url=f"http://127.0.0.1:{direct.server_port}",
                timeout_seconds=1.0,
            )
            resp = client.call({"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
            self.assertEqual(resp["result"], {"via": "proxy"})
        finally:
            proxy.shutdown()
            proxy.server_close()
            direct.shutdown()
            direct.server_close()

    def test_call_falls_back_to_direct_when_proxy_is_killed_mid_run(self):
        proxy = make_server("proxy")
        direct = make_server("direct")
        client = OutpostClient(
            proxy_url=f"http://127.0.0.1:{proxy.server_port}",
            direct_url=f"http://127.0.0.1:{direct.server_port}",
            timeout_seconds=1.0,
        )

        first = client.call({"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
        self.assertEqual(first["result"], {"via": "proxy"})

        proxy.shutdown()
        proxy.server_close()  # actually frees the port so the next attempt fails fast

        with warnings.catch_warnings(record=True) as caught:
            warnings.simplefilter("always")
            second = client.call({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
            self.assertEqual(second["result"], {"via": "direct"})
            self.assertTrue(
                any("falling back to direct connection" in str(w.message) for w in caught),
                "expected a visible fallback warning",
            )

        direct.shutdown()
        direct.server_close()

    def test_call_rejects_oversized_response_body(self):
        # Claude Security finding F20: _post_json read the entire
        # response with resp.read() and no size cap, so a malicious or
        # compromised upstream (notably reachable on the direct-fallback
        # path, where Outpost's own protections are bypassed by design)
        # could force the agent process to buffer an arbitrarily large
        # body. max_response_bytes is tiny here so the test doesn't need
        # to actually transfer megabytes to prove the cap holds.
        def make_oversized_server():
            class Handler(BaseHTTPRequestHandler):
                def do_POST(self):
                    length = int(self.headers.get("Content-Length", 0))
                    self.rfile.read(length)
                    body = json.dumps(
                        {"jsonrpc": "2.0", "id": 1, "result": "a" * 1000}
                    ).encode("utf-8")
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(body)

                def log_message(self, fmt, *args):
                    pass

            server = HTTPServer(("127.0.0.1", 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            return server

        proxy = make_oversized_server()
        try:
            client = OutpostClient(
                proxy_url=f"http://127.0.0.1:{proxy.server_port}",
                direct_url="http://127.0.0.1:1",  # unused: proxy responds, no fallback needed
                timeout_seconds=1.0,
                max_response_bytes=100,
            )
            with self.assertRaises(Exception):
                client.call({"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
        finally:
            proxy.shutdown()
            proxy.server_close()

    def test_call_accepts_response_at_cap(self):
        proxy = make_server("proxy")
        try:
            client = OutpostClient(
                proxy_url=f"http://127.0.0.1:{proxy.server_port}",
                direct_url="http://127.0.0.1:1",
                timeout_seconds=1.0,
                max_response_bytes=10 * 1024 * 1024,
            )
            resp = client.call({"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
            self.assertEqual(resp["result"], {"via": "proxy"})
        finally:
            proxy.shutdown()
            proxy.server_close()


if __name__ == "__main__":
    unittest.main()
