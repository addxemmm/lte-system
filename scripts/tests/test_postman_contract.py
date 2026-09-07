#!/usr/bin/env python3
"""Offline contract checks for the committed Postman collections."""

from __future__ import annotations

import json
import shutil
import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
FULL_PATH = ROOT / "postman" / "lte-system.postman_collection.json"
SMOKE_PATH = ROOT / "postman" / "lte-system.readonly-smoke.postman_collection.json"

EXPECTED_OPERATIONS = {
    "POST /api/v1/cell",
    "GET /api/v1/cell",
    "DELETE /api/v1/cell",
    "GET /api/v1/network",
    "GET /api/v1/ues",
    "GET /api/v1/ues/{imsi}",
    "GET /api/v1/subscribers",
    "POST /api/v1/subscribers",
    "GET /api/v1/subscribers/{imsi}",
    "PATCH /api/v1/subscribers/{imsi}",
    "DELETE /api/v1/subscribers/{imsi}",
    "GET /api/v1/diagnostics/connectivity",
    "POST /api/v1/crack/jobs",
    "GET /api/v1/crack/result",
    "POST /api/v1/config/subscribers",
    "POST /api/v1/config/wordlist",
    "GET /api/v1/captures/{id}",
    "POST /api/v1/simcards",
    "GET /api/v1/profile",
    "GET /api/v1/health",
}

SMOKE_OPERATIONS = {
    "GET /api/v1/cell",
    "GET /api/v1/network",
    "GET /api/v1/ues",
    "GET /api/v1/subscribers",
    "GET /api/v1/profile",
    "GET /api/v1/diagnostics/connectivity",
}

REMOVED_PATHS = {
    "/start", "/stop", "/basicinfo", "/crackapn", "/getcrackresult",
    "/userupload", "/passwordupload", "/getfile", "/writesim",
    "/healthz", "/status", "/profile", "/api/v1/ue",
}


def load(path: Path) -> dict:
    with path.open(encoding="utf-8") as stream:
        return json.load(stream)


def requests(collection: dict) -> list[dict]:
    out: list[dict] = []

    def walk(items: list[dict]) -> None:
        for item in items:
            if "request" in item:
                out.append(item)
            walk(item.get("item", []))

    walk(collection["item"])
    return out


def request_url(item: dict) -> str:
    value = item["request"]["url"]
    if isinstance(value, dict):
        value = value["raw"]
    return value


def operation(item: dict) -> str:
    path = request_url(item).removeprefix("{{base_url}}").split("?", 1)[0]
    path = path.replace("/{{subscriber_imsi}}", "/{imsi}")
    if path == "/api/v1/captures/lte-data":
        path = "/api/v1/captures/{id}"
    return f"{item['request']['method']} {path}"


def script(item: dict) -> str:
    lines: list[str] = []
    for event in item.get("event", []):
        if event.get("listen") == "test":
            lines.extend(event.get("script", {}).get("exec", []))
    return "\n".join(lines)


class PostmanContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.full = load(FULL_PATH)
        cls.smoke = load(SMOKE_PATH)
        cls.full_requests = requests(cls.full)
        cls.smoke_requests = requests(cls.smoke)

    def test_full_collection_covers_contract_without_legacy_routes(self) -> None:
        self.assertEqual(len(self.full_requests), 21)
        self.assertEqual({operation(item) for item in self.full_requests}, EXPECTED_OPERATIONS)
        for item in self.full_requests:
            path = request_url(item).removeprefix("{{base_url}}").split("?", 1)[0]
            self.assertNotIn(path, REMOVED_PATHS)

    def test_default_start_and_placeholders_are_non_destructive(self) -> None:
        values = {entry["key"]: entry.get("value", "") for entry in self.full["variable"]}
        self.assertEqual(values["base_url"], "http://HOST:8081")
        self.assertEqual(values["token"], "")
        for key in ("first_dns", "subscriber_imsi", "subscriber_key", "subscriber_opc", "subscriber_sqn"):
            self.assertTrue(values[key].startswith("REPLACE_WITH_"), key)

        starts = [item for item in self.full_requests if operation(item) == "POST /api/v1/cell"]
        self.assertEqual(len(starts), 2)
        default = next(item for item in starts if "default:" in item["name"])
        first_install = next(item for item in starts if "first installation" in item["name"])
        self.assertEqual(default["request"]["body"]["raw"], "{}")
        template = first_install["request"]["body"]["raw"]
        self.assertIn('"network": "auto"', template)
        self.assertIn('"dns": "{{first_dns}}"', template)
        self.assertNotIn("8.8.8.8", template)
        self.assertNotIn("192.168.100.1", template)
        self.assertNotIn('"network": "eth0"', template)

    def test_safe_json_gets_have_specific_assertions(self) -> None:
        expected_names = {
            "GET /api/v1/cell", "GET /api/v1/network", "GET /api/v1/ues",
            "GET /api/v1/ues/{imsi}", "GET /api/v1/subscribers",
            "GET /api/v1/subscribers/{imsi}", "GET /api/v1/profile",
            "GET /api/v1/health", "GET /api/v1/diagnostics/connectivity",
        }
        by_name = {item["name"]: item for item in self.full_requests}
        for name in expected_names:
            body = script(by_name[name])
            self.assertIn("pm.test(", body, name)
            self.assertIn("request_id", body, name)
        self.assertIn("internet_reachable", script(by_name["GET /api/v1/network"]))
        self.assertIn("sessions.length", script(by_name["GET /api/v1/ues"]))
        self.assertIn("diagnostic_busy", script(by_name["GET /api/v1/diagnostics/connectivity"]))

    def test_readonly_smoke_is_exactly_six_safe_gets(self) -> None:
        self.assertEqual(len(self.smoke_requests), 6)
        self.assertEqual({operation(item) for item in self.smoke_requests}, SMOKE_OPERATIONS)
        for item in self.smoke_requests:
            self.assertEqual(item["request"]["method"], "GET")
            self.assertNotIn("body", item["request"])
            self.assertIn("pm.test(", script(item))
        text = json.dumps(self.smoke, ensure_ascii=False).lower()
        for forbidden in ("/health", "/captures", "/crack", "/simcards", "/config/", '"delete"', '"patch"', '"post"'):
            self.assertNotIn(forbidden, text)
        self.assertIn("does not", self.smoke["info"]["description"].lower())
        self.assertIn("internet", self.smoke["info"]["description"].lower())

    def test_diagnostic_assertion_accepts_only_exact_200_or_429_contract(self) -> None:
        node = shutil.which("node")
        if node is None:
            self.skipTest("node is not installed")
        diagnostic = next(
            item for item in self.smoke_requests
            if operation(item) == "GET /api/v1/diagnostics/connectivity"
        )
        diagnostic_script = script(diagnostic)
        harness = f"""
const SCRIPT = {json.dumps(diagnostic_script)};
function execute(code, body) {{
  const failures = [];
  const pm = {{
    response: {{code: code, json: () => body}},
    test: (name, fn) => {{ try {{ fn(); }} catch (err) {{ failures.push(name + ': ' + err.message); }} }},
    expect: (actual) => ({{to: {{eql: (expected) => {{
      if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error('assertion failed');
    }}}}}}),
  }};
  try {{ eval(SCRIPT); }} catch (err) {{ failures.push('script: ' + err.message); }}
  return failures;
}}
const chap = {{state: 'unknown', reason: 'capture_incomplete', capture: {{state: 'incomplete', incomplete: true, snapshot_size_bytes: 24, complete_packets: 0}}, scan_complete: false, s1ap_observed: false, chap_observed: false, pap_observed: false}};
const layers = {{registration: {{}}, pdn: {{}}, chap, user_plane: {{}}, dns: {{}}, network: {{}}}};
const cases = [
  [200, {{code: 0, request_id: 'r1', data: layers}}, true],
  [429, {{code: 42901, request_id: 'r2', data: {{reason: 'diagnostic_busy'}}}}, true],
  [200, {{code: 50001, request_id: 'r3', data: layers}}, false],
  [200, {{code: 0, request_id: 'r4', data: {{registration: {{}}}}}}, false],
  [429, {{code: 42901, request_id: 'r5', data: {{reason: 'wrong'}}}}, false],
  [500, {{code: 50001, request_id: 'r6', data: {{}}}}, false],
  [200, {{code: 0, request_id: 'r7', data: {{...layers, chap: {{...chap, scan_complete: true}}}}}}, false],
  [200, {{code: 0, request_id: 'r8', data: {{...layers, chap: {{...chap, password: 'SYNTHETIC_FORBIDDEN'}}}}}}, false],
  [200, {{code: 0, request_id: 'r9', data: {{...layers, chap: {{...chap, capture: {{state: 'incomplete', complete_packets: -1}}}}}}}}, false],
  [200, {{code: 0, request_id: 'r10', data: {{...layers, chap: {{...chap, state: 'not_observed', reason: 'no_chap_frames_observed', capture: {{state: 'present'}}, scan_complete: true}}}}}}, true],
];
for (const [code, body, shouldPass] of cases) {{
  const passed = execute(code, body).length === 0;
  if (passed !== shouldPass) process.exit(1);
}}
"""
        result = subprocess.run(
            [node, "-e", harness], capture_output=True, text=True, timeout=5, check=False
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
