#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import io
import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
STYLE_SCRIPT = SCRIPT_DIR / "style_assets.py"
LINT_SCRIPT = SCRIPT_DIR / "lint_card.py"
SPEC = importlib.util.spec_from_file_location("style_assets", STYLE_SCRIPT)
assert SPEC and SPEC.loader
style_assets = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = style_assets
SPEC.loader.exec_module(style_assets)


class StyleAssetsTest(unittest.TestCase):
    def test_catalog_contains_all_presets(self) -> None:
        styles = style_assets.style_catalog()
        ids = {item["style_id"] for item in styles}
        self.assertEqual(len(styles), 19)
        self.assertEqual(
            len([item for item in styles if item["family"] == "custom-visual"]),
            11,
        )
        self.assertEqual(
            len([item for item in styles if item["family"] == "dashboard"]),
            6,
        )
        for expected in {
            "default-semantic",
            "modern-minimal",
            "handdrawn-dark",
            "wellness-planner",
            "claude-chrome",
            "dashboard-monitoring",
        }:
            self.assertIn(expected, ids)

    def test_asset_catalog_and_manifests_are_complete(self) -> None:
        assets = style_assets.asset_catalog()
        counts = {
            family: sum(item["family"] == family for item in assets)
            for family in ("header", "doodle", "phosphor")
        }
        self.assertEqual(counts, {"header": 5, "doodle": 153, "phosphor": 48})
        self.assertEqual(style_assets.verify_assets(), [])

    def test_custom_tokens_are_card_compatible_rgba(self) -> None:
        token_document = style_assets.style_tokens("handdrawn-dark")
        colors = token_document["config"]["style"]["color"]
        self.assertIn("wb_handdrawn_dark_canvas", colors)
        self.assertIn("wb_handdrawn_dark_on_accent", colors)
        self.assertIn("wb_handdrawn_dark_muted_text", colors)
        for modes in colors.values():
            self.assertRegex(modes["light_mode"], r"^rgba\(")
            self.assertRegex(modes["dark_mode"], r"^rgba\(")
            self.assertNotIn("#", modes["light_mode"])

    def test_starter_text_contrast_is_accessible(self) -> None:
        for style in style_assets.custom_styles():
            prefix = style_assets.token_prefix(style["style_id"])
            colors = style_assets.custom_theme_tokens(style)
            ratio = style_assets.contrast_on(
                colors[f"{prefix}_canvas"]["light_mode"],
                colors[f"{prefix}_muted_text"]["light_mode"],
            )
            self.assertGreaterEqual(ratio, 4.5, style["style_id"])

    def test_copy_preserves_asset_bytes(self) -> None:
        source = style_assets.get_asset("doodle:trend-up.safe16")
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "trend.png"
            args = type(
                "Args",
                (),
                {
                    "asset_id": "doodle:trend-up.safe16",
                    "output": output,
                    "force": False,
                },
            )()
            with redirect_stdout(io.StringIO()):
                style_assets.command_copy(args)
            self.assertEqual(Path(source["path"]).read_bytes(), output.read_bytes())

    def test_modern_and_custom_starters_pass_strict_lint_with_upload(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            for style_id in ("modern-minimal", "handdrawn-dark"):
                output = Path(temp_dir) / f"{style_id}.json"
                result = subprocess.run(
                    [
                        sys.executable,
                        str(STYLE_SCRIPT),
                        "starter",
                        style_id,
                        "--output",
                        str(output),
                        "--title",
                        "技能素材库已接入",
                        "--summary",
                        "预置主题和本地素材可以直接选择",
                        "--detail",
                        "发送前由 CLI 上传并替换图片 token",
                        "--meta",
                        "真实验证样例",
                    ],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                lint = subprocess.run(
                    [
                        sys.executable,
                        str(LINT_SCRIPT),
                        "--strict",
                        "--upload-images",
                        str(output),
                    ],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                self.assertEqual(
                    lint.returncode,
                    0,
                    f"{style_id}: stdout={lint.stdout}\nstderr={lint.stderr}",
                )

    def test_dashboard_starter_requires_real_content_in_strict_mode(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            draft = Path(temp_dir) / "draft.json"
            result = subprocess.run(
                [
                    sys.executable,
                    str(STYLE_SCRIPT),
                    "starter",
                    "dashboard-monitoring",
                    "--output",
                    str(draft),
                ],
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            lint = subprocess.run(
                [sys.executable, str(LINT_SCRIPT), "--strict", str(draft)],
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertNotEqual(lint.returncode, 0)
            self.assertIn("placeholder", lint.stderr)

    def test_dashboard_presets_have_distinct_information_structures(self) -> None:
        signatures = set()
        with tempfile.TemporaryDirectory() as temp_dir:
            for preset in style_assets.DASHBOARD_PRESETS:
                output = Path(temp_dir) / f"{preset['style_id']}.json"
                result = subprocess.run(
                    [
                        sys.executable,
                        str(STYLE_SCRIPT),
                        "starter",
                        preset["style_id"],
                        "--output",
                        str(output),
                        "--title",
                        "业务状态",
                        "--summary",
                        "当前结论",
                        "--detail",
                        "结构化洞察",
                        "--meta",
                        "真实口径",
                        "--metric",
                        "指标 A=10",
                        "--metric",
                        "指标 B=20",
                    ],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                card = json.loads(output.read_text(encoding="utf-8"))
                signature = tuple(
                    element["tag"] for element in card["body"]["elements"][:-1]
                )
                signatures.add(signature)
            self.assertEqual(len(signatures), 6)

    def test_starter_rejects_cross_family_icon(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            cases = [
                (
                    "modern-minimal",
                    "doodle:magic-wand.safe16",
                    "只接受 phosphor",
                ),
                (
                    "handdrawn-dark",
                    "phosphor:phosphor_regular_warning_safety-emergency",
                    "只接受 doodle",
                ),
            ]
            for style_id, icon, expected in cases:
                output = Path(temp_dir) / f"{style_id}.json"
                result = subprocess.run(
                    [
                        sys.executable,
                        str(STYLE_SCRIPT),
                        "starter",
                        style_id,
                        "--icon",
                        icon,
                        "--output",
                        str(output),
                    ],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                self.assertEqual(result.returncode, 2)
                self.assertIn(expected, result.stderr)
                self.assertFalse(output.exists())

    def test_copy_refuses_overwrite_without_force(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "existing.png"
            shutil.copy2(
                style_assets.get_asset("doodle:trend-up.safe16")["path"],
                output,
            )
            args = type(
                "Args",
                (),
                {
                    "asset_id": "doodle:trend-up.safe16",
                    "output": output,
                    "force": False,
                },
            )()
            with self.assertRaisesRegex(ValueError, "输出已存在"):
                style_assets.command_copy(args)

    def test_force_copy_replaces_symlink_without_touching_target(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir) / "unrelated.txt"
            target.write_text("keep me", encoding="utf-8")
            output = Path(temp_dir) / "asset.png"
            output.symlink_to(target)
            args = type(
                "Args",
                (),
                {
                    "asset_id": "doodle:trend-up.safe16",
                    "output": output,
                    "force": True,
                },
            )()
            with redirect_stdout(io.StringIO()):
                style_assets.command_copy(args)
            self.assertFalse(output.is_symlink())
            self.assertEqual(target.read_text(encoding="utf-8"), "keep me")
            self.assertEqual(
                output.read_bytes(),
                Path(style_assets.get_asset("doodle:trend-up.safe16")["path"]).read_bytes(),
            )


if __name__ == "__main__":
    unittest.main()
