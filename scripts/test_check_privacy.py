#!/usr/bin/env python3
"""check_privacy.py 的离线回归。样本用字符串拼接构造，避免本文件自身被扫描命中。"""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import check_privacy  # noqa: E402

AT = "@"
INTERNAL = "byte" + "dance.com"


def rules(text: str, path: str = "doc.md") -> list[str]:
    return [f.rule for f in check_privacy.scan_text(path, text)]


class PrivacyScanTest(unittest.TestCase):
    def test_internal_email(self) -> None:
        self.assertEqual(rules("联系 zhang.san" + AT + INTERNAL), ["internal-email"])
        self.assertEqual(rules("mailto:li" + AT + "lark" + "suite.com"), ["internal-email"])
        self.assertEqual(rules("user" + AT + "example.com"), [])
        # 仅提到域名、不构成邮箱地址
        self.assertEqual(rules("禁止出现 `" + AT + INTERNAL + "`"), [])
        # URL userinfo 不是邮箱
        self.assertEqual(rules("https://user:pass" + AT + "open.fei" + "shu.cn/x"), [])

    def test_tenant_domain(self) -> None:
        self.assertEqual(rules("https://acme" + ".fei" + "shu.cn/docx/abc"), ["tenant-domain"])
        self.assertEqual(rules("https://acme" + ".lark" + "office.com/wiki/x"), ["tenant-domain"])
        for ok in ("open", "www", "accounts", "xxx", "example", "sample", "applink"):
            self.assertEqual(rules(f"https://{ok}.fei" + "shu.cn/x"), [], ok)
        self.assertEqual(rules("https://fei" + "shu.cn/x"), [])

    def test_known_example_only_in_its_file(self) -> None:
        line = "禁止带企业前缀（如 `byte" + "dance.fei" + "shu.cn`）"
        self.assertEqual(rules(line, "CLAUDE.md"), [])
        self.assertEqual(rules(line, "README.md"), ["tenant-domain"])

    def test_app_id(self) -> None:
        self.assertEqual(rules("app_id: cli_" + "a1b2c3d4e5f60718"), ["app-id"])
        self.assertEqual(rules("app_id: cli_xxx"), [])
        self.assertEqual(rules("app_id: cli_" + "aaaaaaaaaaaaaaaa"), [])

    def test_tokens(self) -> None:
        real_user = "u-" + "3Kd9fQx7LmP2vR8sT1wY5zA0bC4eG6hJ"
        self.assertEqual(rules("token=" + real_user), ["user-token"])
        self.assertEqual(rules("t-" + "g1044ghJK2LMN3pqr4STU5vwx6YZa7b8"), ["user-token"])
        for placeholder in ("u-xxx", "u-" + "secret-token-super-private-9999", "u-" + "abcdefghijklmnopqrstuvwxyz", "t-" + "xxxxxxxxxxxxxxxxxxxxxxxx"):
            self.assertEqual(rules(placeholder), [], placeholder)
        self.assertEqual(rules("app_secret: " + "Zq8Wk2Lm9Np4Rs7Tv1Xy3Ab6Cd0Ef5Gh"), ["app-secret"])
        self.assertEqual(rules("--app-secret " + "Zq8Wk2Lm9Np4Rs7Tv1Xy3Ab6Cd0Ef5Gh"), ["app-secret"])
        self.assertEqual(rules("app_secret: xxx"), [])
        jwt = "eyJ" + "hbGciOiJIUzI1NiJ9" + "." + "eyJzdWIiOiIxMjM0NTY3ODkwIn0" + "." + "dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
        self.assertEqual(rules(jwt), ["jwt"])
        self.assertEqual(rules("ghp_" + "A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q7r8"), ["github-token"])

    def test_allow_marker(self) -> None:
        self.assertEqual(rules("cli_" + "a1b2c3d4e5f60718  # privacy-scan: allow"), [])

    def test_cli_exit_codes(self) -> None:
        script = Path(check_privacy.__file__)
        email = "wang" + AT + INTERNAL
        with tempfile.TemporaryDirectory() as tmp:
            leak = Path(tmp) / "leak.md"
            leak.write_text("owner: " + email + "\n", encoding="utf-8")
            clean = Path(tmp) / "clean.md"
            clean.write_text("owner: user" + AT + "example.com\n", encoding="utf-8")
            # 绝对路径同样可扫描（ROOT / 绝对路径 == 绝对路径）
            bad = subprocess.run([sys.executable, str(script), str(leak)], capture_output=True, text=True)
            good = subprocess.run([sys.executable, str(script), str(clean)], capture_output=True, text=True)
        self.assertEqual(bad.returncode, 1, bad.stderr)
        self.assertIn("internal-email", bad.stderr)
        self.assertNotIn(email, bad.stderr)  # 输出已脱敏
        self.assertEqual(good.returncode, 0, good.stderr)


if __name__ == "__main__":
    unittest.main()
