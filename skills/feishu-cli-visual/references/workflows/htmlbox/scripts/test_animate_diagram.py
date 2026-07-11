#!/usr/bin/env python3
"""Regression tests for the self-contained animated diagram generator."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import animate_diagram


def _pattern(*, nodes=None, edges=None, timeline=None):
    return {
        "title": "Test",
        "nodes": nodes or [],
        "edges": edges or {},
        "timeline": timeline or [{"caption": "ok"}],
    }


class RenderTests(unittest.TestCase):
    def test_caption_uses_safe_dom_builder(self):
        html = animate_diagram.render(
            _pattern(timeline=[{
                "caption": '<b onclick="window.pwned=1">bad</b><code>ok</code><img src=x onerror="window.pwned=1">'
            }])
        )
        self.assertNotIn("capEl.innerHTML", html)
        self.assertIn("document.createTextNode", html)
        self.assertIn("document.createElement(tagName)", html)

    def test_token_duration_follows_current_step(self):
        html = animate_diagram.render(_pattern())
        self.assertIn("Math.min(dur * 0.85, 1400)", html)
        self.assertNotIn("(1500 / SPEED) * 0.85", html)

    def test_pause_and_reduced_motion_are_wired(self):
        html = animate_diagram.render(_pattern())
        self.assertIn('.viz.paused svg.diagram .node.active', html)
        self.assertIn("prefers-reduced-motion: reduce", html)
        self.assertIn('matchMedia("(prefers-reduced-motion: reduce)")', html)


class ViewBoxTests(unittest.TestCase):
    def test_store_circle_uses_its_real_vertical_extent(self):
        pattern = _pattern(nodes=[{
            "id": "store", "x": 100, "y": 100, "w": 140, "h": 54,
            "label": "Store", "kind": "store",
        }])
        self.assertEqual("76 33 188 188", animate_diagram.compute_viewbox(pattern))

    def test_quadratic_curve_extremum_is_included(self):
        pattern = _pattern(
            nodes=[
                {"id": "a", "x": 100, "y": 100, "w": 100, "h": 40, "label": "A"},
                {"id": "b", "x": 400, "y": 100, "w": 100, "h": 40, "label": "B"},
            ],
            edges={"a-b": {"from": "a", "to": "b", "curve": 300}},
        )
        x, y, width, height = map(float, animate_diagram.compute_viewbox(pattern).split())
        self.assertLessEqual(x, 76)
        self.assertLessEqual(y, 76)
        self.assertGreaterEqual(x + width, 524)
        self.assertGreaterEqual(y + height, 294)


if __name__ == "__main__":
    unittest.main()
