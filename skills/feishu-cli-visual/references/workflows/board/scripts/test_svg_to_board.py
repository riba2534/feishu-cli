#!/usr/bin/env python3
"""SVG 视口和复合节点回归；不依赖 whiteboard-cli 或远端服务。"""
import contextlib
import copy
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import svg_to_board


class ViewboxTests(unittest.TestCase):
    def test_parse_full_viewbox_and_dimensions(self):
        with tempfile.TemporaryDirectory() as tmp:
            svg = Path(tmp) / "drawing.svg"
            for attrs, expected in [
                ('viewBox="100 200 300 400"', (100, 200, 300, 400)),
                ('viewBox="-100 -200 300 400"', (-100, -200, 300, 400)),
                ('width="300px" height="400px"', (0, 0, 300, 400)),
                ('viewBox="0 0 nan 400"', None),
            ]:
                with self.subTest(attrs=attrs):
                    svg.write_text(f'<svg {attrs}></svg>')
                    self.assertEqual(svg_to_board.parse_viewbox_from_svg(svg), expected)

    def test_nonzero_origins_preserve_composite_child_coordinates(self):
        for origin in (100, -100):
            node = {"type": "composite_shape", "x": origin + 20, "y": origin + 20,
                    "width": 20, "height": 20,
                    "composite_shape": {"type": "round_rect", "children": [
                        {"type": "text_shape", "x": 2, "y": 3, "width": 10, "height": 10}]}}
            expected = copy.deepcopy(node)
            with self.subTest(origin=origin), contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(svg_to_board.step3_trim_overflow([node], 100, 100, False, origin, origin), [expected])

    def test_clipping_and_outside_filter_use_actual_bounds(self):
        nodes = [
            {"x": 90, "y": 120, "width": 30, "height": 20},
            {"x": 190, "y": 120, "width": 30, "height": 20},
            {"x": 10, "y": 10, "width": 20, "height": 20},
        ]
        with contextlib.redirect_stdout(io.StringIO()):
            result = svg_to_board.step3_trim_overflow(nodes, 100, 100, False, 100, 100)
        self.assertEqual([(x["x"], x["width"]) for x in result], [(100, 20), (190, 10)])

    def test_dimension_override_has_zero_origin(self):
        self.assertEqual(svg_to_board.parse_viewbox_arg("1600x900"), (0, 0, 1600, 900))
        with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            svg_to_board.parse_viewbox_arg("0x900")

    def test_translator_array_output_is_supported(self):
        node = {"type": "composite_shape", "x": 10, "y": 10, "width": 20, "height": 20}
        def run(cmd):
            Path(cmd[cmd.index("-o") + 1]).write_text(json.dumps([node]))
            return 0, "", ""
        with mock.patch.object(svg_to_board.shutil, "which", return_value="whiteboard-cli"), \
                mock.patch.object(svg_to_board, "run", side_effect=run), \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(svg_to_board.step1_translate("drawing.svg", False), [node])


class UploadResultTests(unittest.TestCase):
    def test_only_complete_integer_count_confirms_batch(self):
        nodes = [{"type": "composite_shape"}, {"type": "text_shape"}]
        cases = [
            ('{"count":2,"node_ids":["node_1","node_2"]}', 2, False),
            ('log line\n{"count":2}', 2, False),
            ('{"count":1}', 1, True),
            ('{"count":0}', 0, True),
            ('{"count":-1}', 0, True),
            ('{"count":3}', 0, True),
            ('{"count":1.5}', 0, True),
            ('{"count":"2"}', 0, True),
            ('{"count":true}', 0, True),
            ('{"count":null}', 0, True),
            ('{"node_ids":[]}', 0, True),
            ('not JSON', 0, True),
            ('{"count":', 0, True),
        ]
        for output, expected_count, failed in cases:
            log = io.StringIO()
            with self.subTest(output=output), \
                    mock.patch.object(svg_to_board, "run", return_value=(0, output, "")) as run, \
                    contextlib.redirect_stdout(log):
                result = svg_to_board.step4_upload(nodes, "board_fixture", "feishu-cli", 300, 0)
            self.assertEqual(result, (expected_count, [(0, 2)] if failed else []))
            run.assert_called_once()  # 结果不确定也不能重传，避免重复创建。
            if failed:
                self.assertNotIn("✓", log.getvalue())
                self.assertIn("未自动重传", log.getvalue())

    def test_each_batch_carries_deterministic_client_token(self):
        nodes = [{"type": "composite_shape", "x": i} for i in range(3)]
        calls = []
        def run(cmd):
            calls.append(cmd)
            with open(cmd[4], encoding="utf-8") as batch:
                return 0, '{"count":%d}' % len(json.load(batch)), ""
        for _ in range(2):  # 原样重跑：同画板、同内容 → 同 token，服务端返回已落地节点
            with mock.patch.object(svg_to_board, "run", side_effect=run), \
                    contextlib.redirect_stdout(io.StringIO()):
                svg_to_board.step4_upload(copy.deepcopy(nodes), "board_fixture", "feishu-cli", 2, 0)
        tokens = [cmd[cmd.index("--client-token") + 1] for cmd in calls]
        self.assertEqual(len(tokens), 4)
        self.assertEqual(tokens[:2], tokens[2:])
        self.assertNotEqual(tokens[0], tokens[1])
        self.assertTrue(all(len(t) >= 10 for t in tokens))
        self.assertNotEqual(tokens[0], svg_to_board.batch_client_token("other_board", nodes[:2]))

    def test_partial_upload_main_exits_nonzero(self):
        node = {"type": "composite_shape", "x": 10, "y": 10, "width": 20, "height": 20}
        with tempfile.TemporaryDirectory() as tmp:
            svg = Path(tmp) / "drawing.svg"
            svg.write_text('<svg viewBox="0 0 100 100"></svg>')
            for response, verified in (('{"count":0}', True), ('not JSON', True), ('{"count":1}', False)):
                log = io.StringIO()
                with self.subTest(response=response), \
                        mock.patch.object(sys, "argv", ["svg_to_board.py", str(svg), "board_fixture"]), \
                        mock.patch.object(svg_to_board.shutil, "which", return_value="feishu-cli"), \
                        mock.patch.object(svg_to_board, "step1_translate", return_value=[node.copy()]), \
                        mock.patch.object(svg_to_board, "run", return_value=(0, response, "")) as run, \
                        mock.patch.object(svg_to_board, "step5_verify", return_value=verified), \
                        contextlib.redirect_stdout(log), self.assertRaises(SystemExit) as exc:
                    svg_to_board.main()
                self.assertEqual(exc.exception.code, 3)
                self.assertIn("========== 未完成", log.getvalue())
                self.assertIn("不要直接整批重传", log.getvalue())
                run.assert_called_once()


class ReadbackTests(unittest.TestCase):
    def test_readback_failures_are_not_success(self):
        cases = [
            (1, "", "unavailable"),
            (0, "not JSON", ""),
            (0, '{"data":{}}', ""),
            (0, '{"data":{"nodes":"invalid"}}', ""),
            (0, '{"data":{"nodes":[null]}}', ""),
            (0, '{"data":{"nodes":[]}}', ""),
        ]
        for response in cases:
            with self.subTest(response=response), \
                    mock.patch.object(svg_to_board, "run", return_value=response) as run, \
                    contextlib.redirect_stdout(io.StringIO()):
                self.assertFalse(svg_to_board.step5_verify("board_fixture", "feishu-cli", 1))
            run.assert_called_once()

    def test_existing_nodes_do_not_imply_duplicates(self):
        nodes = [{"type": "text_shape"}, {"type": "composite_shape"}]
        for response_nodes in (nodes, {"node_1": nodes[0], "node_2": nodes[1]}):
            log = io.StringIO()
            with self.subTest(nodes=response_nodes), \
                    mock.patch.object(svg_to_board, "run", return_value=(0, json.dumps({"data": {"nodes": response_nodes}}), "")), \
                    contextlib.redirect_stdout(log):
                self.assertTrue(svg_to_board.step5_verify("board_fixture", "feishu-cli", 1))
            self.assertIn("含已有节点", log.getvalue())
            self.assertNotIn("delete --all", log.getvalue())
            self.assertNotIn("可能发生翻倍", log.getvalue())


if __name__ == "__main__":
    unittest.main()
