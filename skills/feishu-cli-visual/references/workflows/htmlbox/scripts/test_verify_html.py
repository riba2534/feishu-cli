#!/usr/bin/env python3
"""验证器的成功/失败契约，不依赖已安装浏览器。"""
import base64
import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import verify_html

PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")


class VerifyTests(unittest.TestCase):
    def run_case(self, failure=None, errors=None, create_screenshot=True, nodes=1):
        with tempfile.TemporaryDirectory() as tmp:
            page = Path(tmp).resolve() / "page.html"
            page.write_text("<html><body>test</body></html>")
            # 旧截图不能让本次截图失败蒙混过关。
            page.with_name("page.verify.png").write_bytes(PNG)
            calls = []
            def call(*args):
                calls.append(args)
                if args[0] == failure:
                    raise verify_html.BrowserError("dependency failed")
                if args[0] == "eval":
                    return {"result": {"nodes": nodes, "status": "", "url": page.as_uri(), "ready": "complete"}}
                if args[0] == "errors":
                    return {"errors": errors or []}
                if args[0] == "console":
                    return {"messages": []}
                if args[0] == "screenshot" and create_screenshot:
                    Path(args[1]).write_bytes(PNG)
                return {}
            with mock.patch.object(verify_html.shutil, "which", return_value="browser"), \
                    mock.patch.object(verify_html.Browser, "run", side_effect=call), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                try:
                    result = verify_html.verify(page, 0)
                    self.assertEqual(page.with_name("page.verify.png").read_bytes(), PNG)
                    return result
                finally:
                    self.assertEqual(calls[-1], ("close",))
                    self.assertNotIn(("close", "--all"), calls)

    def test_success_with_real_screenshot_bytes(self):
        self.assertEqual(self.run_case(), 0)

    def test_plain_css_without_canvas_is_allowed(self):
        self.assertEqual(self.run_case(nodes=0), 0)

    def test_browser_stage_failures_propagate(self):
        for stage in ("open", "wait", "eval", "errors", "console", "screenshot"):
            with self.subTest(stage=stage), self.assertRaises(verify_html.BrowserError):
                self.run_case(failure=stage)

    def test_missing_screenshot_is_not_replaced_by_old_file(self):
        with self.assertRaisesRegex(verify_html.BrowserError, "截图未生成"):
            self.run_case(create_screenshot=False)

    def test_page_error_returns_failure(self):
        self.assertEqual(self.run_case(errors=[{"message": "boom"}]), 1)

    def test_browser_response_must_succeed_and_be_json(self):
        for result in (subprocess.CompletedProcess([], 1, "", "unavailable"),
                       subprocess.CompletedProcess([], 0, "not json", ""),
                       subprocess.CompletedProcess([], 0, '{"success":false,"error":"bad"}', ""),
                       subprocess.CompletedProcess([], 0, '{"success":true}', "")):
            with self.subTest(result=result), mock.patch.object(verify_html.subprocess, "run", return_value=result):
                with self.assertRaises(verify_html.BrowserError):
                    verify_html.Browser().run("open", "file:///page.html")

    def test_sessions_are_unique_and_passed_explicitly(self):
        one, two = verify_html.Browser(), verify_html.Browser()
        self.assertNotEqual(one.session, two.session)
        result = subprocess.CompletedProcess([], 0, json.dumps({"success": True, "data": {}}), "")
        with mock.patch.object(verify_html.subprocess, "run", return_value=result) as run:
            one.run("close")
        self.assertEqual(run.call_args.args[0], ["agent-browser", "--session", one.session, "--json", "close"])


if __name__ == "__main__":
    unittest.main()
