import pathlib
import re
import shutil
import subprocess
import tempfile
import textwrap
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
STATIC = ROOT / "internal" / "webui" / "static"


class WebUIFrontendTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.node = shutil.which("node")

    def test_es_modules_parse_as_modules(self):
        if not self.node:
            self.skipTest("node is not installed")
        with tempfile.TemporaryDirectory() as temp_dir:
            for name in ("app.js", "api.js", "i18n.js"):
                module = pathlib.Path(temp_dir) / name.replace(".js", ".mjs")
                module.write_bytes((STATIC / name).read_bytes())
                result = subprocess.run(
                    [self.node, "--check", str(module)],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(result.returncode, 0, f"{name}: {result.stderr}")

    def test_csp_and_markup_have_no_inline_script_or_style_handlers(self):
        html = (STATIC / "index.html").read_text(encoding="utf-8")
        self.assertIn("script-src 'self'", html)
        self.assertIn("style-src 'self'", html)
        self.assertNotRegex(html, r"(?i)\sstyle\s*=")
        self.assertNotRegex(html, r"(?i)\son[a-z]+\s*=")
        self.assertNotRegex(html, r"(?is)<script(?![^>]*\bsrc=)[^>]*>\s*\S")

    def test_ui_does_not_reference_excluded_sensitive_routes(self):
        source = "\n".join(
            (STATIC / name).read_text(encoding="utf-8")
            for name in ("app.js", "api.js", "index.html")
        )
        for route in ("/crack", "/simcards", "/captures", "/config/wordlist", "/config/subscribers"):
            self.assertNotIn(route, source)
        self.assertIn('requestStart({}, "saved")', source)
        self.assertIn('requestStart(payload, "explicit")', source)
        self.assertIn('returnValue === "confirm"', source)

    def test_api_client_headers_abort_deadline_and_auth_generation(self):
        if not self.node:
            self.skipTest("node is not installed")
        api_uri = (STATIC / "api.js").as_uri()
        script = textwrap.dedent(
            f"""
            import assert from 'node:assert/strict';
            import {{ createApiClient }} from {api_uri!r};

            const envelope = (data={{}}) => JSON.stringify({{code:0,message:'ok',data,request_id:'rid'}});
            let seen;
            const basic = createApiClient({{fetchImpl: async (url, options) => {{
              seen = {{url, options}};
              return new Response(envelope({{running:false}}), {{status:200,headers:{{'Content-Type':'application/json'}}}});
            }}}});
            basic.setToken('TOKEN');
            await basic.startCell({{}});
            assert.equal(seen.url, '/api/v1/cell');
            assert.equal(seen.options.method, 'POST');
            assert.equal(seen.options.headers.get('Authorization'), 'Bearer TOKEN');
            assert.equal(seen.options.headers.get('X-LTE-UI'), '1');

            const external = new AbortController();
            const aborting = createApiClient({{fetchImpl: (_url, options) => new Promise((_resolve, reject) => {{
              options.signal.addEventListener('abort', () => reject(new DOMException('aborted','AbortError')), {{once:true}});
            }})}});
            const aborted = aborting.cell({{signal:external.signal}});
            external.abort('visibility');
            await assert.rejects(aborted, (error) => error.kind === 'aborted');

            const deadline = createApiClient({{fetchImpl: async (_url, options) => ({{
              ok:true, status:200, statusText:'OK', headers:new Headers({{'Content-Type':'application/json'}}),
              json:() => new Promise((_resolve, reject) => options.signal.addEventListener('abort', () => reject(new DOMException('aborted','AbortError')), {{once:true}}))
            }})}});
            await assert.rejects(deadline.request('/cell', {{timeout:25}}), (error) => error.kind === 'timeout');

            let releaseOld;
            let authEvents = 0;
            const events = new EventTarget();
            events.addEventListener('lte:auth-required', () => authEvents++);
            const delayed = createApiClient({{eventTarget:events, fetchImpl: (url, options) => {{
              const auth = options.headers.get('Authorization');
              if (auth === 'Bearer OLD') return new Promise((resolve) => {{ releaseOld = () => resolve(new Response(JSON.stringify({{code:40101,message:'bad',data:null}}), {{status:401,headers:{{'Content-Type':'application/json'}}}})); }});
              return Promise.resolve(new Response(envelope({{auth}}), {{status:200,headers:{{'Content-Type':'application/json'}}}}));
            }}}});
            delayed.setToken('OLD');
            const oldRequest = delayed.cell();
            delayed.setToken('NEW');
            releaseOld();
            await assert.rejects(oldRequest, (error) => error.status === 401);
            assert.equal(delayed.hasToken(), true);
            assert.equal(authEvents, 0);
            const latest = await delayed.cell();
            assert.equal(latest.data.auth, 'Bearer NEW');
            """
        )
        result = subprocess.run(
            [self.node, "--input-type=module", "--eval", script],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_six_hash_routes_and_health_is_user_triggered(self):
        app = (STATIC / "app.js").read_text(encoding="utf-8")
        for route in ("overview", "cell", "ues", "subscribers", "diagnostics", "settings"):
            self.assertIn(f'["{route}",', app)
        self.assertEqual(len(re.findall(r"api\.health\(", app)), 1)
        self.assertIn("runHealth", app)


if __name__ == "__main__":
    unittest.main()
