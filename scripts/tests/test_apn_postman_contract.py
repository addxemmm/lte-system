#!/usr/bin/env python3
"""Offline synthetic checks for schema-versioned UE policy assertions."""
import copy
import json
import shutil
import subprocess
import unittest

from test_postman_contract import FULL_PATH, SMOKE_PATH, load, operation, requests, script


class APNPostmanContractTest(unittest.TestCase):
    def test_list_policy_assertions_accept_v1_v2_and_reject_contradictions(self):
        node = shutil.which("node")
        if node is None:
            self.skipTest("node is not installed")
        base = dict(apn_source="pdn_request", requested_apn="internet",
                    selected_apn="internet", apn_validated=True)
        normal = dict(base, access_policy="normal", access_reason="apn_match")
        restricted = dict(base, requested_apn="wrong-apn", apn_validated=False,
                          access_policy="restricted", access_reason="apn_mismatch")
        deny = dict(apn_source="omitted", apn_validated=False,
                    access_policy="deny", access_reason="session_unavailable")

        def current(version, session):
            return dict(state="current", cell_state="running", schema_version=version,
                        sessions=[session], count=1)

        cases = [
            ("legacy", current(1, base), True),
            ("normal", current(2, normal), True),
            ("case-insensitive", current(2, dict(normal, requested_apn="INTERNET")), True),
            ("omitted", current(2, dict(normal, apn_source="omitted", requested_apn="",
                                        access_reason="apn_omitted")), True),
            ("restricted", current(2, restricted), True),
            ("deny", current(2, deny), True),
            ("unavailable", dict(state="missing", sessions=[], count=0), True),
            ("unknown version", current(3, normal), False),
            ("v1 with policy fields", current(1, restricted), False),
            ("v2 without policy fields", current(2, base), False),
            ("restricted validated", current(2, dict(restricted, apn_validated=True)), False),
            ("restricted matching", current(2, dict(restricted, requested_apn="INTERNET")), False),
            ("restricted omitted", current(2, dict(restricted, apn_source="omitted")), False),
            ("normal mismatch", current(2, dict(normal, requested_apn="wrong")), False),
            ("deny selected", current(2, dict(deny, selected_apn="internet")), False),
            ("deny null selected", current(2, dict(deny, selected_apn=None)), False),
            ("unknown policy", current(2, dict(restricted, access_policy="allow")), False),
            ("unknown reason", current(2, dict(restricted, access_reason="unknown")), False),
        ]
        for field in ("access_policy", "access_reason", "apn_validated"):
            missing = copy.deepcopy(restricted)
            missing.pop(field)
            cases.append(("missing " + field, current(2, missing), False))
            cases.append(("null " + field, current(2, dict(restricted, **{field: None})), False))
        missing_version = current(2, restricted)
        missing_version.pop("schema_version")
        cases.append(("missing version", missing_version, False))

        for path in (FULL_PATH, SMOKE_PATH):
            item = next(i for i in requests(load(path)) if operation(i) == "GET /api/v1/ues")
            harness = """
const SCRIPT = __SCRIPT__;
const CASES = __CASES__;
for (const [name, data, expected] of CASES) {
  const failures = [];
  const pm = {
    response: {code: 200, json: () => ({code: 0, request_id: 'synthetic', data})},
    test: (name, fn) => { try { fn(); } catch (e) { failures.push(name + ': ' + e.message); } },
    expect: value => ({to: {eql: expected => {
      if (JSON.stringify(value) !== JSON.stringify(expected)) throw Error('assertion failed');
    }}}),
  };
  try { eval(SCRIPT); } catch (e) { failures.push(String(e)); }
  if ((failures.length === 0) !== expected) {
    console.error(name, failures); process.exit(1);
  }
}
console.log('APN_POSTMAN_CASES=' + CASES.length);
""".replace("__SCRIPT__", json.dumps(script(item))).replace("__CASES__", json.dumps(cases))
            with self.subTest(collection=path.name):
                result = subprocess.run([node, "-e", harness], capture_output=True,
                                        text=True, timeout=10, check=False)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
