#!/usr/bin/env python3
"""fetch_chat_history CLI 定位逻辑的回归测试。"""

from __future__ import annotations

import os
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
            root = Path(tmp)
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
            root = Path(tmp)
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


if __name__ == "__main__":
    unittest.main()
