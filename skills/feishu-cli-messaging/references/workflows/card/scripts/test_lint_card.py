#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("lint_card.py")
SPEC = importlib.util.spec_from_file_location("lint_card", SCRIPT)
assert SPEC and SPEC.loader
lint_card = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = lint_card
SPEC.loader.exec_module(lint_card)


def issue_codes(
    card: object,
    *,
    allow_placeholders: bool = False,
    upload_images: bool = False,
    base_dir: Path | None = None,
) -> set[str]:
    linter = lint_card.CardLinter(
        allow_placeholders=allow_placeholders,
        upload_images=upload_images,
        base_dir=base_dir,
    )
    return {issue.code for issue in linter.lint(card)}


class CardLinterTest(unittest.TestCase):
    def test_minimal_card_allows_optional_text_align(self) -> None:
        card = {
            "schema": "2.0",
            "body": {"elements": [{"tag": "markdown", "content": "hello"}]},
        }
        self.assertEqual(issue_codes(card), set())

    def test_header_and_markdown_enums(self) -> None:
        card = {
            "schema": "2.0",
            "header": {"title": {"tag": "markdown", "content": "**bad**"}},
            "body": {
                "elements": [{"tag": "markdown", "content": "hello", "text_align": "justify"}]
            },
        }
        codes = issue_codes(card)
        self.assertIn("header_title", codes)
        self.assertIn("text_align", codes)

    def test_config_summary_only_accepts_nonempty_content(self) -> None:
        card = {
            "schema": "2.0",
            "config": {
                "summary": {
                    "content": "",
                    "i18n_content": {"zh_cn": "错误字段"},
                }
            },
            "body": {"elements": [{"tag": "markdown", "content": "hello"}]},
        }
        codes = issue_codes(card)
        self.assertIn("summary_property", codes)
        self.assertIn("summary_content", codes)

        card["config"]["summary"] = {"content": "有效摘要"}
        self.assertNotIn("summary_property", issue_codes(card))
        self.assertNotIn("summary_content", issue_codes(card))

    def test_rejects_textarea_and_nested_table(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "textarea", "rows": 3},
                    {
                        "tag": "interactive_container",
                        "elements": [{"tag": "table", "columns": [{"name": "a"}], "rows": []}],
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("invalid_textarea", codes)
        self.assertIn("table_nesting", codes)

    def test_person_list_uses_id(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "person", "user_id": "ou_real", "user_id_type": "open_id"},
                    {"tag": "person_list", "persons": [{"user_id": "ou_real"}]},
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("person_id_type", codes)
        self.assertIn("person_list_id", codes)

    def test_table_data_type_and_multiline_input(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "table",
                        "columns": [{"name": "owner", "data_type": "person_list"}],
                        "rows": [{"owner": []}],
                    },
                    {"tag": "input", "input_type": "text", "rows": 3},
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("table_data_type", codes)
        self.assertIn("input_multiline", codes)

    def test_empty_card_todo_and_invalid_urls_are_blocked(self) -> None:
        empty = {"schema": "2.0", "body": {"elements": []}}
        self.assertIn("body_empty", issue_codes(empty))

        card = {
            "schema": "2.0",
            "card_link": {"url": "TODO"},
            "body": {
                "elements": [
                    {"tag": "markdown", "content": "TODO: 补负责人"},
                    {
                        "tag": "button",
                        "text": {"tag": "plain_text", "content": "查看"},
                        "behaviors": [{"type": "open_url", "default_url": "not-a-url"}],
                    },
                    {
                        "tag": "button",
                        "text": {"tag": "plain_text", "content": "历史链接"},
                        "action_type": "link",
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("placeholder", codes)
        self.assertIn("invalid_url", codes)
        self.assertIn("legacy_link_url", codes)

    def test_images_require_real_tokens_and_meaningful_alt(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "img", "img_key": "missing.png", "alt": {}},
                    {
                        "tag": "img_combination",
                        "combination_mode": "double",
                        "img_list": [{"img_key": "img_v2_xxx1"}],
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("local_image_missing", codes)
        self.assertIn("img_alt", codes)
        self.assertIn("img_combination_count", codes)
        self.assertIn("placeholder", codes)

    def test_rejects_non_image_keys_and_css_style_layout_fields(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "img",
                        "img_key": "file_v2_realish",
                        "alt": {"tag": "plain_text", "content": "错误 token"},
                    },
                    {
                        "tag": "column_set",
                        "width": "fill",
                        "margin-top": "20px",
                        "background_style": "default",
                        "flex_mode": "stretch",
                        "horizontal_spacing": "8px",
                        "columns": [],
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("img_key_format", codes)
        self.assertIn("column_set_width", codes)
        self.assertIn("css_layout_property", codes)

    def test_local_card_images_require_upload_flag_and_accept_existing_assets(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            base_dir = Path(temp_dir)
            (base_dir / "cover.png").write_bytes(b"png")
            (base_dir / "icon.png").write_bytes(b"png")
            card = {
                "schema": "2.0",
                "body": {
                    "elements": [
                        {
                            "tag": "img",
                            "img_key": "cover.png",
                            "alt": {"tag": "plain_text", "content": "封面"},
                        },
                        {
                            "tag": "img_combination",
                            "combination_mode": "double",
                            "img_list": [
                                {"img_key": "icon.png"},
                                {"img_key": "img_v2_existing"},
                            ],
                        },
                    ]
                },
            }

            without_flag = issue_codes(card, base_dir=base_dir)
            self.assertIn("local_image_upload", without_flag)
            with_flag = issue_codes(card, upload_images=True, base_dir=base_dir)
            self.assertNotIn("local_image_upload", with_flag)
            self.assertNotIn("local_image_missing", with_flag)

    def test_local_card_image_expands_user_home(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            home_dir = Path(temp_dir)
            (home_dir / "cover.tiff").write_bytes(b"tiff")
            card = {
                "schema": "2.0",
                "body": {
                    "elements": [
                        {
                            "tag": "img",
                            "img_key": "~/cover.tiff",
                            "alt": {"tag": "plain_text", "content": "封面"},
                        }
                    ]
                },
            }
            original_home = os.environ.get("HOME")
            os.environ["HOME"] = str(home_dir)
            try:
                codes = issue_codes(card, upload_images=True)
            finally:
                if original_home is None:
                    os.environ.pop("HOME", None)
                else:
                    os.environ["HOME"] = original_home
            self.assertNotIn("local_image_missing", codes)
            self.assertNotIn("local_image_upload", codes)
            self.assertNotIn("img_key", codes)

    def test_nested_form_submit_button_is_not_reported_as_actionless(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "form",
                        "name": "feedback",
                        "elements": [
                            {
                                "tag": "column_set",
                                "columns": [
                                    {
                                        "tag": "column",
                                        "elements": [
                                            {
                                                "tag": "button",
                                                "name": "submit",
                                                "form_action_type": "submit",
                                                "text": {
                                                    "tag": "plain_text",
                                                    "content": "提交",
                                                },
                                            }
                                        ],
                                    }
                                ],
                            }
                        ],
                    }
                ]
            },
        }
        self.assertNotIn("button_action", issue_codes(card))

    def test_display_only_interactive_container_is_allowed_without_fake_hover(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "interactive_container",
                        "background_style": "grey-50",
                        "elements": [{"tag": "markdown", "content": "展示型 surface"}],
                    }
                ]
            },
        }
        self.assertNotIn("interactive_action", issue_codes(card))
        card["body"]["elements"][0]["hover_tips"] = {
            "tag": "plain_text",
            "content": "点我",
        }
        self.assertIn("interactive_action", issue_codes(card))

    def test_collapsible_panel_header_requires_light_surface(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "collapsible_panel",
                        "expanded": False,
                        "header": {
                            "title": {
                                "tag": "markdown",
                                "content": "<font color='blue'>**核心观点**</font>",
                            },
                            "background_color": "blue",
                        },
                        "elements": [{"tag": "markdown", "content": "详情"}],
                    }
                ]
            },
        }
        self.assertIn("collapsible_header_surface", issue_codes(card))

        card["body"]["elements"][0]["header"]["background_color"] = "white"
        self.assertNotIn("collapsible_header_surface", issue_codes(card))

        card["body"]["elements"][0]["header"]["background_color"] = (
            "rgba(247, 249, 252, 1)"
        )
        self.assertNotIn("collapsible_header_surface", issue_codes(card))

        card["body"]["elements"][0]["header"]["background_color"] = (
            "rgba(51, 112, 255, 1)"
        )
        self.assertIn("collapsible_header_surface", issue_codes(card))

        del card["body"]["elements"][0]["header"]["background_color"]
        card["body"]["elements"][0]["background_color"] = "purple"
        self.assertIn("collapsible_header_surface", issue_codes(card))

    def test_local_markdown_image_requires_existing_file_and_upload_mode(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [{"tag": "markdown", "content": "![状态图](status.png)"}]
            },
        }
        with tempfile.TemporaryDirectory() as directory:
            base_dir = Path(directory)
            missing = {
                issue.code
                for issue in lint_card.CardLinter(base_dir=base_dir).lint(card)
            }
            self.assertIn("local_image_missing", missing)

            (base_dir / "status.png").write_bytes(b"not-a-real-image")
            without_flag = {
                issue.code
                for issue in lint_card.CardLinter(base_dir=base_dir).lint(card)
            }
            with_flag = {
                issue.code
                for issue in lint_card.CardLinter(
                    base_dir=base_dir,
                    upload_images=True,
                ).lint(card)
            }
            self.assertIn("local_image_upload", without_flag)
            self.assertNotIn("local_image_upload", with_flag)

    def test_placeholders_can_only_pass_in_template_mode(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "button",
                        "type": "primary",
                        "text": {"tag": "plain_text", "content": "__ACTION__"},
                        "behaviors": [{"type": "open_url", "default_url": "https://example.com"}],
                    }
                ]
            },
        }
        self.assertIn("placeholder", issue_codes(card))
        self.assertIn("example_url", issue_codes(card))
        self.assertNotIn("placeholder", issue_codes(card, allow_placeholders=True))
        self.assertNotIn("example_url", issue_codes(card, allow_placeholders=True))

    def test_placeholder_object_key_blocks_send_candidate(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "chart",
                        "chart_spec": {
                            "type": "bar",
                            "__REPLACE_WITH_REAL_CHART_DATA__": True,
                            "data": {"values": [{"name": "sample", "value": 0}]},
                            "xField": "name",
                            "yField": "value",
                        },
                    }
                ]
            },
        }
        self.assertIn("placeholder", issue_codes(card))
        self.assertNotIn("placeholder", issue_codes(card, allow_placeholders=True))

    def test_openapi_wrapper_and_duplicate_element_id(self) -> None:
        inner = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "markdown", "content": "A", "element_id": "same"},
                    {"tag": "markdown", "content": "B", "element_id": "same"},
                ]
            },
        }
        wrapper = {"msg_type": "interactive", "content": json.dumps(inner)}
        codes = issue_codes(wrapper)
        self.assertIn("element_id_duplicate", codes)
        self.assertIn("wrapper_not_sendable", codes)

    def test_media_components_require_core_fields(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "audio", "bogus": True},
                    {
                        "tag": "audio",
                        "background_style": "default",
                        "file_key": "clip.mp3",
                    },
                    {"tag": "video", "cover": {}},
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("audio_background_style", codes)
        self.assertIn("audio_file_key", codes)
        self.assertIn("video_file_key", codes)
        self.assertIn("image_key", codes)

    def test_valid_media_avatar_fallback_and_icons_pass(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {
                        "tag": "audio",
                        "background_style": "default",
                        "file_key": "file_v2_real_audio",
                    },
                    {
                        "tag": "video",
                        "file_key": "file_v2_real_video",
                        "cover": {"img_key": "img_v2_real_cover"},
                    },
                    {"tag": "avatar", "size": "medium", "user_id": "ou_real"},
                    {
                        "tag": "fallback_text",
                        "text": {
                            "tag": "plain_text",
                            "i18n": {
                                "zh_cn": "暂时无法展示",
                                "en_us": "Temporarily unavailable",
                            },
                        },
                    },
                    {"tag": "custom_icon", "img_key": "img_v2_real_icon"},
                    {"tag": "standard_icon", "token": "info_outlined"},
                    {"tag": "ud_icon", "token": "info", "style": "outlined"},
                ]
            },
        }
        self.assertEqual(issue_codes(card), set())

    def test_avatar_custom_size_is_consistent(self) -> None:
        missing = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "avatar", "size": "custom", "user_id": "ou_real"}
                ]
            },
        }
        self.assertIn("avatar_custom_size", issue_codes(missing))

        missing["body"]["elements"][0]["custom_size"] = 40
        self.assertNotIn("avatar_custom_size", issue_codes(missing))

    def test_selectors_validate_options_and_selected_values(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "select_static", "options": []},
                    {
                        "tag": "multi_select_static",
                        "options": [
                            {
                                "text": {"tag": "plain_text", "content": "通过"},
                                "value": "pass",
                            },
                            {
                                "text": {"tag": "plain_text", "content": "重复"},
                                "value": "pass",
                            },
                        ],
                        "selected_values": ["missing"],
                    },
                    {
                        "tag": "select_img",
                        "style": "invalid",
                        "options": [
                            {"img_key": "cover.png"},
                            {"img_key": "img_v2_real", "value": "cover"},
                        ],
                        "selected_values": ["cover", "other"],
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("select_options", codes)
        self.assertIn("select_option_duplicate", codes)
        self.assertIn("selected_values_unknown", codes)
        self.assertIn("select_img_style", codes)
        self.assertIn("image_key_upload_scope", codes)
        self.assertIn("select_img_value", codes)
        self.assertIn("select_img_single", codes)

    def test_picker_initial_value_matches_component_type_and_format(self) -> None:
        card = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "picker_date", "initial_date": "2026/07/30"},
                    {"tag": "picker_time", "initial_date": "2026-07-30"},
                    {
                        "tag": "picker_datetime",
                        "initial_datetime": "2026-07-30T25:00",
                    },
                ]
            },
        }
        codes = issue_codes(card)
        self.assertIn("picker_initial_format", codes)
        self.assertIn("picker_initial_field", codes)

        valid = {
            "schema": "2.0",
            "body": {
                "elements": [
                    {"tag": "picker_date", "initial_date": "2026-07-30"},
                    {"tag": "picker_time", "initial_time": "23:59"},
                    {
                        "tag": "picker_datetime",
                        "initial_datetime": "2026-07-30 23:59",
                    },
                ]
            },
        }
        self.assertEqual(issue_codes(valid), set())

    def test_all_project_templates_pass_template_mode(self) -> None:
        templates = Path(__file__).parents[1] / "templates"
        for path in sorted(templates.glob("*.json")):
            with self.subTest(template=path.name):
                card = json.loads(path.read_text(encoding="utf-8"))
                errors = [
                    issue
                    for issue in lint_card.CardLinter(allow_placeholders=True).lint(card)
                    if issue.level == "error"
                ]
                self.assertEqual(errors, [])


if __name__ == "__main__":
    unittest.main()
