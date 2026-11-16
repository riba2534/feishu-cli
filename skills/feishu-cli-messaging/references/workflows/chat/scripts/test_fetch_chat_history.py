#!/usr/bin/env python3
"""fetch_chat_history CLI 定位逻辑的回归测试。"""

from __future__ import annotations

import os
import contextlib
import io
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import fetch_chat_history


class FindCliTests(unittest.TestCase):
    def _make_repo(self, root: Path) -> Path:
        (root / "go.mod").write_text(
            "module github.com/riba2534/feishu-cli\n\ngo 1.21\n",
            encoding="utf-8",
        )
        script = root / "skills/feishu-cli-messaging/references/workflows/chat/scripts/fetch_chat_history.py"
        script.parent.mkdir(parents=True)
        script.touch()
        return script

    def _make_executable(self, path: Path, mtime_ns: int) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/bin/sh\n", encoding="utf-8")
        path.chmod(0o755)
        os.utime(path, ns=(mtime_ns, mtime_ns))

    def test_explicit_path_has_highest_priority(self):
        self.assertEqual("/custom/feishu-cli", fetch_chat_history.find_cli("/custom/feishu-cli"))

    def test_chooses_newest_executable_from_repo_root_or_bin(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            script = self._make_repo(root)
            root_cli = root / "feishu-cli"
            bin_cli = root / "bin/feishu-cli"
            self._make_executable(root_cli, 100)
            self._make_executable(bin_cli, 200)

            with mock.patch.object(fetch_chat_history, "__file__", str(script)), \
                    mock.patch.object(fetch_chat_history.shutil, "which", return_value="/path/feishu-cli"):
                self.assertEqual(str(bin_cli), fetch_chat_history.find_cli(None))

            os.utime(root_cli, ns=(300, 300))
            with mock.patch.object(fetch_chat_history, "__file__", str(script)):
                self.assertEqual(str(root_cli), fetch_chat_history.find_cli(None))

    def test_ignores_non_executable_repo_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            script = self._make_repo(root)
            root_cli = root / "feishu-cli"
            root_cli.write_text("not executable\n", encoding="utf-8")
            bin_cli = root / "bin/feishu-cli"
            self._make_executable(bin_cli, 100)

            with mock.patch.object(fetch_chat_history, "__file__", str(script)):
                self.assertEqual(str(bin_cli), fetch_chat_history.find_cli(None))

    def test_installed_skill_without_repo_marker_falls_back_to_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            script = Path(tmp) / ".claude/skills/feishu-cli-messaging/scripts/fetch_chat_history.py"
            script.parent.mkdir(parents=True)
            script.touch()

            with mock.patch.object(fetch_chat_history, "__file__", str(script)), \
                    mock.patch.object(fetch_chat_history.shutil, "which", return_value="/usr/local/bin/feishu-cli"):
                self.assertEqual("/usr/local/bin/feishu-cli", fetch_chat_history.find_cli(None))


class ResolveUserInfoTests(unittest.TestCase):
    def test_user_info_does_not_receive_explicit_user_token(self):
        with mock.patch.object(
            fetch_chat_history,
            "run_cli_json",
            return_value={"name": "测试用户"},
        ) as run_cli_json:
            names = fetch_chat_history.resolve_with_user_info(
                "/path/feishu-cli",
                ["ou_xxx"],
                "u-explicit-token",
            )

        self.assertEqual({"ou_xxx": "测试用户"}, names)
        run_cli_json.assert_called_once_with(
            "/path/feishu-cli",
            ["user", "info", "ou_xxx", "-o", "json"],
        )



class FetchIntegrityTests(unittest.TestCase):
    def history(self, responses):
        with mock.patch.object(fetch_chat_history, "run_cli_json", side_effect=responses), \
                contextlib.redirect_stdout(io.StringIO()):
            return fetch_chat_history.fetch_history("cli", "oc_test", "chat", 1, 2, None)

    def test_cli_failure_is_not_empty_history(self):
        result = subprocess.CompletedProcess([], 1, "", "读取接口失败")
        with mock.patch.object(fetch_chat_history.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(fetch_chat_history.FetchError, "读取接口失败"):
                fetch_chat_history.run_cli_json("cli", ["msg", "history"])

    def test_invalid_json_fails(self):
        result = subprocess.CompletedProcess([], 0, "not json", "")
        with mock.patch.object(fetch_chat_history.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(fetch_chat_history.FetchError, "JSON"):
                fetch_chat_history.run_cli_json("cli", ["msg", "history"])

    def test_mid_pagination_failure_propagates(self):
        with self.assertRaisesRegex(fetch_chat_history.FetchError, "读取失败"):
            self.history([{"items": [{"message_id": "a"}], "has_more": True, "page_token": "next"},
                          fetch_chat_history.FetchError("读取失败")])

    def test_empty_and_repeated_cursors_fail(self):
        for token in ("", "same"):
            with self.subTest(token=token), self.assertRaisesRegex(fetch_chat_history.FetchError, "游标"):
                self.history([{"items": [], "has_more": True, "page_token": token}] * 2)

    def test_page_cap_is_not_success(self):
        with mock.patch.object(fetch_chat_history, "MAX_HISTORY_PAGES", 1):
            with self.assertRaisesRegex(fetch_chat_history.FetchError, "页上限"):
                self.history([{"items": [], "has_more": True, "page_token": "next"}])

    def test_overlapping_pages_deduplicate(self):
        items, names = self.history([
            {"items": [{"message_id": "a"}], "has_more": True, "page_token": "next"},
            {"items": [{"message_id": "a"}, {"message_id": "b"}], "has_more": False},
        ])
        self.assertEqual([x["message_id"] for x in items], ["a", "b"])

    def test_thread_pagination_checks_and_pascal_case(self):
        page = {"Items": [{"message_id": "a"}], "HasMore": True, "PageToken": "same"}
        with mock.patch.object(fetch_chat_history, "run_cli_json", return_value=page):
            with self.assertRaisesRegex(fetch_chat_history.FetchError, "游标"):
                fetch_chat_history.fetch_thread("cli", "omt_test", None)
        with mock.patch.object(fetch_chat_history, "MAX_THREAD_PAGES", 1), \
                mock.patch.object(fetch_chat_history, "run_cli_json", return_value=page):
            with self.assertRaisesRegex(fetch_chat_history.FetchError, "页上限"):
                fetch_chat_history.fetch_thread("cli", "omt_test", None)
        with mock.patch.object(fetch_chat_history, "run_cli_json", return_value={"Items": [], "HasMore": False}):
            self.assertEqual(fetch_chat_history.fetch_thread("cli", "omt_test", None), ([], {}))

    def test_error_envelope_cannot_be_an_empty_page(self):
        with self.assertRaisesRegex(fetch_chat_history.FetchError, "缺少 items"):
            self.history([{"code": 999}])

    def test_optional_name_failure_still_degrades(self):
        with mock.patch.object(fetch_chat_history, "run_cli_json", side_effect=fetch_chat_history.FetchError("41050")):
            self.assertEqual(fetch_chat_history.resolve_with_user_info("cli", ["ou_test"], None), {})

    def test_export_preserves_known_bot_names(self):
        items = [{"message_id": "m1", "create_time": "1", "msg_type": "text",
                  "sender": {"id_type": "app_id", "id": "cli_a"}, "body": {"content": '{"text":"hello"}'}},
                 {"message_id": "m2", "create_time": "2", "msg_type": "text",
                  "sender": {"id_type": "app_id", "id": "cli_b"}, "body": {"content": '{"text":"world"}'}}]
        with tempfile.TemporaryDirectory() as tmp, \
                mock.patch.object(sys, "argv", ["fetch", "oc_test", "--cli", "cli", "--no-thread", "--output-dir", tmp]), \
                mock.patch.object(fetch_chat_history, "run_cli_json", return_value={
                    "items": items, "sender_names": {"cli_a": "Build bot", "cli_b": "Alert bot"}}), \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(fetch_chat_history.main(), 0)
            names = json.loads((Path(tmp) / "names.json").read_text())
            self.assertEqual(names, {"cli_a": "Build bot", "cli_b": "Alert bot"})
            timeline = (Path(tmp) / "timeline.txt").read_text()
            self.assertIn("Build bot", timeline)
            self.assertIn("Alert bot", timeline)


if __name__ == "__main__":
    unittest.main()
