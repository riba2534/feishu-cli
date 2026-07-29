#!/usr/bin/env python3
"""对飞书 Card JSON 2.0 做离线结构与发送前质量检查。

这是面向 feishu-cli 工作流的保守 linter，不冒充飞书服务端的完整 Schema 校验。
它覆盖项目最常见、且可以离线确定的错误：V1 字段、非法嵌套、未替换占位符、
重复 element_id、无意义交互、媒体与选择器核心字段、表格字段错配和本地图片发送前置条件。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


PLACEHOLDER_RE = re.compile(
    r"__[A-Z][A-Z0-9_]*__|TODO_[A-Z][A-Z0-9_]*|(?<![A-Z0-9_])TODO(?![A-Z0-9_])",
    re.IGNORECASE,
)
ELEMENT_ID_RE = re.compile(r"^[A-Za-z][A-Za-z0-9_]{0,19}$")
IMG_KEY_RE = re.compile(r"^img_[A-Za-z0-9._-]+$")
MARKDOWN_IMAGE_RE = re.compile(r"!\[[^\]]*]\(([^)]+)\)")
GENERIC_ID_RE = re.compile(
    r"^(?:(?:ou|oc|om|omt|on|cli)_(?:x+|\d+|example|test)|"
    r"(?:img|file)_v[23]_(?:x+\d*|example|test))$",
    re.IGNORECASE,
)

ROOT_KEYS = {"schema", "config", "card_link", "header", "body", "fallback_card"}
KNOWN_TAGS = {
    "audio",
    "avatar",
    "button",
    "chart",
    "checker",
    "collapse_divider",
    "collapsible_panel",
    "column",
    "column_set",
    "custom_icon",
    "date_picker",
    "div",
    "fallback_text",
    "form",
    "hr",
    "img",
    "img_combination",
    "input",
    "interactive_container",
    "lark_md",
    "markdown",
    "multi_select_person",
    "multi_select_static",
    "overflow",
    "person",
    "person_list",
    "picker_date",
    "picker_datetime",
    "picker_time",
    "plain_text",
    "select_img",
    "select_person",
    "select_static",
    "standard_icon",
    "table",
    "text_tag",
    "ud_icon",
    "video",
}
CONTAINER_TAGS = {"column_set", "collapsible_panel", "form", "interactive_container"}
TABLE_DATA_TYPES = {"text", "lark_md", "options", "number", "persons", "date", "markdown"}
CSS_LAYOUT_KEYS = {
    "margin-top",
    "margin-right",
    "margin-bottom",
    "margin-left",
    "padding-top",
    "padding-right",
    "padding-bottom",
    "padding-left",
}
LIGHT_COLLAPSIBLE_HEADER_BACKGROUNDS = {
    "default",
    "neutral",
    "transparent",
    "white",
    "grey",
    "gray",
}
SATURATED_COLLAPSIBLE_HEADER_BACKGROUNDS = {
    "black",
    "blue",
    "wathet",
    "turquoise",
    "indigo",
    "green",
    "lime",
    "yellow",
    "orange",
    "red",
    "carmine",
    "purple",
    "violet",
}
LIGHT_COLOR_LEVEL_RE = re.compile(
    r"^(?:neutral|gr[ae]y|blue|wathet|turquoise|indigo|green|lime|yellow|orange|red|carmine|purple|violet)-(?:50|100)$",
    re.IGNORECASE,
)
RGBA_RE = re.compile(
    r"rgba?\(\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,"
    r"\s*(\d+(?:\.\d+)?)\s*(?:,\s*(\d*\.?\d+)\s*)?\)",
    re.IGNORECASE,
)
DATE_RE = re.compile(r"^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])$")
TIME_RE = re.compile(r"^(?:[01]\d|2[0-3]):[0-5]\d$")
DATETIME_RE = re.compile(
    r"^\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01]) "
    r"(?:[01]\d|2[0-3]):[0-5]\d$"
)
AVATAR_SIZES = {"tiny", "small", "medium", "large", "huge", "custom"}
SELECT_TYPES = {"default", "text"}
SELECT_IMG_STYLES = {"default", "laser"}
SELECT_IMG_LAYOUTS = {"stretch", "bisect", "trisect"}
SELECT_IMG_ASPECT_RATIOS = {"16:9", "1:1", "4:3"}
MEDIA_FILE_EXTENSIONS = {
    ".aac",
    ".flac",
    ".m4a",
    ".mov",
    ".mp3",
    ".mp4",
    ".ogg",
    ".wav",
    ".webm",
}


@dataclass(frozen=True)
class Issue:
    level: str
    code: str
    path: str
    message: str


class CardLinter:
    def __init__(
        self,
        *,
        allow_placeholders: bool = False,
        upload_images: bool = False,
        base_dir: Path | None = None,
    ) -> None:
        self.allow_placeholders = allow_placeholders
        self.upload_images = upload_images
        self.base_dir = base_dir or Path.cwd()
        self.issues: list[Issue] = []
        self.element_ids: dict[str, str] = {}
        self.component_count = 0
        self.table_count = 0
        self.local_image_paths: set[str] = set()
        self.style_colors: dict[str, Any] = {}

    def error(self, code: str, path: str, message: str) -> None:
        self.issues.append(Issue("error", code, path, message))

    def warning(self, code: str, path: str, message: str) -> None:
        self.issues.append(Issue("warning", code, path, message))

    def lint(self, data: Any, raw_size: int | None = None) -> list[Issue]:
        card, mode = self._unwrap(data)
        if not isinstance(card, dict):
            self.error("card_type", "$", "卡片顶层必须是 JSON 对象")
            return self.issues
        if mode != "card":
            self.error(
                "wrapper_not_sendable",
                "$",
                "msg send --content-file 需要原始 Card JSON；请从外层请求体提取 card/content",
            )

        compact_size = len(
            json.dumps(card, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        )
        measured_size = raw_size if mode == "card" and raw_size is not None else compact_size
        if measured_size > 30 * 1024:
            self.error(
                "card_size",
                "$",
                f"卡片 JSON 为 {measured_size} 字节，超过 interactive 消息 30 KB 上限",
            )

        for key in card:
            if key not in ROOT_KEYS:
                self.error("root_property", f"$.{key}", f"Card JSON 2.0 不支持顶层属性 {key}")

        if card.get("schema") != "2.0":
            self.error("schema_version", "$.schema", '必须显式设置为字符串 "2.0"')

        config = card.get("config")
        if config is not None and not isinstance(config, dict):
            self.error("config_type", "$.config", "config 必须是对象")
        elif isinstance(config, dict):
            self._lint_config(config)

        card_link = card.get("card_link")
        if card_link is not None:
            if not isinstance(card_link, dict):
                self.error("card_link_type", "$.card_link", "card_link 必须是对象")
            else:
                self._lint_url_fields(card_link, "$.card_link", require_default=True)

        header = card.get("header")
        if header is not None and not isinstance(header, dict):
            self.error("header_type", "$.header", "header 必须是对象")
        elif isinstance(header, dict):
            title = header.get("title")
            if not isinstance(title, dict) or title.get("tag") not in {"plain_text", "lark_md"}:
                self.error(
                    "header_title",
                    "$.header.title",
                    "使用 header 时必须提供 plain_text 或 lark_md 标题",
                )
            self.component_count += sum(
                isinstance(item.get("tag"), str) for item in self._iter_dicts(header)
            )

        body = card.get("body")
        if not isinstance(body, dict):
            self.error("body_required", "$.body", "必须提供 body 对象")
        else:
            elements = body.get("elements")
            if not isinstance(elements, list):
                self.error("body_elements", "$.body.elements", "必须提供 elements 数组")
            elif not elements:
                self.error("body_empty", "$.body.elements", "发送候选不能是空卡片")
            else:
                for index, element in enumerate(elements):
                    self._walk_component(
                        element,
                        f"$.body.elements[{index}]",
                        parent_tag="body",
                        parent_slot="elements",
                        container_depth=0,
                        in_form=False,
                    )

        self._scan_values(card)

        if self.component_count > 200:
            self.error(
                "component_limit",
                "$.body",
                f"共 {self.component_count} 个带 tag 的元素，超过单卡 200 个上限",
            )
        if self.table_count > 5:
            self.error("table_limit", "$.body", f"共 {self.table_count} 个 table，超过单卡 5 个上限")

        return self.issues

    def _unwrap(self, data: Any) -> tuple[Any, str]:
        if not isinstance(data, dict) or data.get("msg_type") != "interactive":
            return data, "card"

        if "card" in data:
            return data["card"], "webhook"

        if "content" in data:
            content = data["content"]
            if not isinstance(content, str):
                self.error(
                    "openapi_content",
                    "$.content",
                    "OpenAPI interactive content 必须是序列化后的 Card JSON 字符串",
                )
                return content, "openapi"
            try:
                return json.loads(content), "openapi"
            except json.JSONDecodeError as exc:
                self.error("openapi_content", "$.content", f"content 不是有效 JSON：{exc}")
                return content, "openapi"

        self.error("interactive_wrapper", "$", "interactive 外层请求体必须包含 card 或 content")
        return data, "wrapper"

    def _lint_config(self, config: dict[str, Any]) -> None:
        style = config.get("style")
        if isinstance(style, dict) and isinstance(style.get("color"), dict):
            self.style_colors = style["color"]
        summary = config.get("summary")
        if summary is not None:
            if not isinstance(summary, dict):
                self.error("summary_type", "$.config.summary", "summary 必须是对象")
            else:
                unknown = sorted(set(summary) - {"content"})
                if unknown:
                    self.error(
                        "summary_property",
                        "$.config.summary",
                        f"summary 只支持 content，不支持：{', '.join(unknown)}",
                    )
                content = summary.get("content")
                if not isinstance(content, str) or not content.strip():
                    self.error(
                        "summary_content",
                        "$.config.summary.content",
                        "summary 必须提供非空 content",
                    )
        if config.get("update_multi") is False:
            self.error(
                "update_multi",
                "$.config.update_multi",
                "Card JSON 2.0 暂只支持共享卡片；删除该字段或设为 true",
            )
        if "wide_screen_mode" in config:
            self.error(
                "v1_config",
                "$.config.wide_screen_mode",
                'V2 使用 width_mode: "fill"，不要使用 wide_screen_mode',
            )
        if "compact_width" in config:
            self.warning(
                "legacy_config",
                "$.config.compact_width",
                '当前 V2 官方结构使用 width_mode；建议改为 width_mode: "compact"',
            )
        width_mode = config.get("width_mode")
        if width_mode is not None and width_mode not in {"default", "compact", "fill"}:
            self.error(
                "width_mode",
                "$.config.width_mode",
                "仅支持 default、compact、fill",
            )

    def _walk_component(
        self,
        node: Any,
        path: str,
        *,
        parent_tag: str,
        parent_slot: str,
        container_depth: int,
        in_form: bool,
    ) -> None:
        if not isinstance(node, dict):
            self.error("component_type", path, "组件必须是 JSON 对象")
            return

        tag = node.get("tag")
        if not isinstance(tag, str) or not tag:
            self.error("component_tag", f"{path}.tag", "组件必须包含非空字符串 tag")
            return

        self.component_count += 1
        if tag not in KNOWN_TAGS and tag not in {"note", "action", "textarea"}:
            self.warning(
                "unknown_tag",
                f"{path}.tag",
                f"项目速查未收录组件 {tag}；发送前请核对当前官方 Card JSON 2.0 文档",
            )

        if tag in {"note", "action"}:
            replacement = "notation 字号的 markdown" if tag == "note" else "button 或 column_set"
            self.error("v1_component", path, f"V2 不支持 {tag}，请改用 {replacement}")
        if tag == "textarea":
            self.error(
                "invalid_textarea",
                path,
                'V2 没有 textarea tag；使用 tag=input、input_type="multiline_text" 和 rows',
            )

        for key in sorted(CSS_LAYOUT_KEYS & node.keys()):
            compact = "margin" if key.startswith("margin-") else "padding"
            self.error(
                "css_layout_property",
                f"{path}.{key}",
                f"Card V2 不支持 CSS 式 {key}；请改用四值 {compact} 字符串",
            )

        next_depth = container_depth + (1 if tag in CONTAINER_TAGS else 0)
        next_in_form = in_form or tag == "form"
        if next_depth > 5:
            self.error("nesting_depth", path, "容器嵌套超过 5 层")

        element_id = node.get("element_id")
        if element_id is not None:
            if not isinstance(element_id, str) or not ELEMENT_ID_RE.fullmatch(element_id):
                self.error(
                    "element_id_format",
                    f"{path}.element_id",
                    "必须字母开头，仅含字母/数字/下划线，且不超过 20 字符",
                )
            elif element_id in self.element_ids:
                self.error(
                    "element_id_duplicate",
                    f"{path}.element_id",
                    f"与 {self.element_ids[element_id]} 重复",
                )
            else:
                self.element_ids[element_id] = f"{path}.element_id"

        if tag == "table":
            self.table_count += 1
            if parent_tag != "body" or parent_slot != "elements":
                self.error("table_nesting", path, "table 不可嵌套，只能直接放在 body.elements")
            self._lint_table(node, path)

        if tag == "form":
            if parent_tag != "body" or parent_slot != "elements":
                self.error("form_nesting", path, "form 不可被其它组件嵌套，只能直接放在 body.elements")
            self._lint_form(node, path)
        if tag == "collapsible_panel":
            self._lint_collapsible_panel(node, path)

        if tag == "column" and (parent_tag != "column_set" or parent_slot != "columns"):
            self.error("column_parent", path, "column 只能出现在 column_set.columns")
        if tag == "column_set":
            if "width" in node:
                self.error(
                    "column_set_width",
                    f"{path}.width",
                    "column_set 不支持 width；用 flex_mode 控制分栏伸缩",
                )
            for required in ("background_style", "flex_mode", "horizontal_spacing"):
                if required not in node:
                    self.error(
                        "column_set_required",
                        f"{path}.{required}",
                        f"column_set 必须显式提供 {required}",
                    )

        if tag == "markdown":
            text_align = node.get("text_align")
            if text_align is not None and text_align not in {"left", "center", "right"}:
                self.error("text_align", f"{path}.text_align", "仅支持 left、center、right")
        elif tag in {"plain_text", "lark_md"}:
            self._lint_text_object(node, path, allowed_tags={tag})
        elif tag == "img":
            self._lint_img(node, path)
        elif tag == "img_combination":
            self._lint_img_combination(node, path)
        elif tag == "audio":
            self._lint_audio(node, path)
        elif tag == "video":
            self._lint_video(node, path)
        elif tag == "avatar":
            self._lint_avatar(node, path)
        elif tag == "fallback_text":
            self._lint_fallback_text(node, path)
        elif tag == "person":
            if not isinstance(node.get("user_id"), str) or not node.get("user_id"):
                self.error("person_id", f"{path}.user_id", "person 必须使用真实 user_id/open_id/union_id")
            if "id" in node:
                self.error("person_legacy_id", f"{path}.id", "person 的 ID 字段是 user_id，不是 id")
            if "user_id_type" in node:
                self.error(
                    "person_id_type",
                    f"{path}.user_id_type",
                    "Card JSON 2.0 person 不使用 user_id_type；服务端按 ID 值识别类型",
                )
        elif tag == "person_list":
            self._lint_person_list(node, path)
        elif tag == "button":
            self._lint_button(node, path, in_form)
        elif tag == "input":
            self._lint_input(node, path)
        elif tag in {
            "select_static",
            "multi_select_static",
            "select_person",
            "multi_select_person",
        }:
            self._lint_select_menu(node, path)
        elif tag == "select_img":
            self._lint_select_img(node, path)
        elif tag in {"date_picker", "picker_date", "picker_time", "picker_datetime"}:
            self._lint_date_picker(node, path)
        elif tag == "chart":
            self._lint_chart(node, path)
        elif tag == "interactive_container":
            self._lint_interactive_container(node, path)
        elif tag in {"custom_icon", "standard_icon", "ud_icon"}:
            self._lint_icon(node, path)

        for key, value in node.items():
            if key == "elements" and isinstance(value, list):
                for index, child in enumerate(value):
                    self._walk_component(
                        child,
                        f"{path}.elements[{index}]",
                        parent_tag=tag,
                        parent_slot="elements",
                        container_depth=next_depth,
                        in_form=next_in_form,
                    )
            elif key == "columns" and tag == "column_set" and isinstance(value, list):
                for index, child in enumerate(value):
                    self._walk_component(
                        child,
                        f"{path}.columns[{index}]",
                        parent_tag=tag,
                        parent_slot="columns",
                        container_depth=next_depth,
                        in_form=next_in_form,
                    )
            elif isinstance(value, dict) and isinstance(value.get("tag"), str):
                self._walk_component(
                    value,
                    f"{path}.{key}",
                    parent_tag=tag,
                    parent_slot=key,
                    container_depth=next_depth,
                    in_form=next_in_form,
                )
            elif isinstance(value, list) and key not in {"elements", "columns"}:
                for index, child in enumerate(value):
                    if isinstance(child, dict) and isinstance(child.get("tag"), str):
                        self._walk_component(
                            child,
                            f"{path}.{key}[{index}]",
                            parent_tag=tag,
                            parent_slot=key,
                            container_depth=next_depth,
                            in_form=next_in_form,
                        )

    def _lint_collapsible_panel(self, node: dict[str, Any], path: str) -> None:
        header = node.get("header")
        if not isinstance(header, dict):
            return
        background = header.get("background_color", node.get("background_color"))
        if background is None:
            return
        if not isinstance(background, str):
            self.warning(
                "collapsible_header_surface",
                f"{path}.header.background_color",
                "折叠标题条背景必须是白色或浅色 surface；语义色应放在标题文字、图标或细边框",
            )
            return

        normalized = background.strip().lower()
        if (
            normalized in LIGHT_COLLAPSIBLE_HEADER_BACKGROUNDS
            or LIGHT_COLOR_LEVEL_RE.fullmatch(normalized)
        ):
            return

        if normalized in SATURATED_COLLAPSIBLE_HEADER_BACKGROUNDS:
            self.warning(
                "collapsible_header_surface",
                f"{path}.header.background_color",
                f"折叠标题条不应使用饱和/深色背景 {background!r}；"
                "改用 white、default 或 *-50/*-100 浅色 surface，并把语义色放到标题文字或图标",
            )
            return

        direct_luminance = self._color_luminance(background)
        if direct_luminance is not None:
            if direct_luminance < 0.72:
                self.warning(
                    "collapsible_header_surface",
                    f"{path}.header.background_color",
                    f"折叠标题条背景 {background!r} 明度过低；"
                    "改用白色/浅色背景，并通过字体颜色表达语义",
                )
            return

        token = self.style_colors.get(background)
        if isinstance(token, dict):
            colors = [
                token.get(mode)
                for mode in ("light_mode", "dark_mode")
                if isinstance(token.get(mode), str)
            ]
            luminances = [
                value
                for value in (self._color_luminance(color) for color in colors)
                if value is not None
            ]
            if luminances and min(luminances) >= 0.72:
                return
            self.warning(
                "collapsible_header_surface",
                f"{path}.header.background_color",
                f"折叠标题条 token {background!r} 不是全模式浅色 surface；"
                "标题条保持白色/浅色，语义色只用于标题文字、图标或细边框",
            )
            return

        self.warning(
            "collapsible_header_surface",
            f"{path}.header.background_color",
            f"无法确认折叠标题条背景 token {background!r} 为浅色；"
            "请改用 white、default、*-50/*-100 或声明可校验的浅色 rgba token",
        )

    @staticmethod
    def _color_luminance(value: str) -> float | None:
        match = RGBA_RE.fullmatch(value.strip())
        if not match:
            return None
        red, green, blue = (float(match.group(index)) / 255 for index in (1, 2, 3))
        alpha = float(match.group(4)) if match.group(4) is not None else 1.0
        red = red * alpha + (1 - alpha)
        green = green * alpha + (1 - alpha)
        blue = blue * alpha + (1 - alpha)

        def linear(channel: float) -> float:
            return channel / 12.92 if channel <= 0.04045 else ((channel + 0.055) / 1.055) ** 2.4

        return 0.2126 * linear(red) + 0.7152 * linear(green) + 0.0722 * linear(blue)

    def _lint_table(self, node: dict[str, Any], path: str) -> None:
        columns = node.get("columns")
        rows = node.get("rows")
        if not isinstance(columns, list) or not columns:
            self.error("table_columns", f"{path}.columns", "table 必须提供非空 columns 数组")
            return
        if len(columns) > 50:
            self.error("table_columns", f"{path}.columns", "table 最多支持 50 列")
        if not isinstance(rows, list):
            self.error("table_rows", f"{path}.rows", "table 必须提供 rows 数组")
            return

        names: list[str] = []
        for index, column in enumerate(columns):
            name = column.get("name") if isinstance(column, dict) else None
            if not isinstance(name, str) or not name:
                self.error("table_column_name", f"{path}.columns[{index}].name", "列必须提供非空 name")
            elif name in names:
                self.error(
                    "table_column_duplicate",
                    f"{path}.columns[{index}].name",
                    f"列名 {name} 重复",
                )
            else:
                names.append(name)
            data_type = column.get("data_type") if isinstance(column, dict) else None
            if data_type not in TABLE_DATA_TYPES:
                self.error(
                    "table_data_type",
                    f"{path}.columns[{index}].data_type",
                    f"必须是当前 V2 枚举之一：{', '.join(sorted(TABLE_DATA_TYPES))}",
                )
        allowed = set(names)
        for index, row in enumerate(rows):
            if not isinstance(row, dict):
                self.error("table_row_type", f"{path}.rows[{index}]", "每行必须是对象")
                continue
            unknown = sorted(set(row) - allowed)
            if unknown:
                self.error(
                    "table_row_key",
                    f"{path}.rows[{index}]",
                    f"行字段未在 columns.name 声明：{', '.join(unknown)}",
                )

        page_size = node.get("page_size")
        if page_size is not None and (
            not isinstance(page_size, int) or isinstance(page_size, bool) or not 1 <= page_size <= 10
        ):
            self.error("table_page_size", f"{path}.page_size", "必须是 1 到 10 的整数")

    def _lint_form(self, node: dict[str, Any], path: str) -> None:
        elements = node.get("elements")
        if not isinstance(elements, list):
            self.error("form_elements", f"{path}.elements", "form 必须提供 elements 数组")
            return

        def contains_tag(value: Any, wanted: set[str]) -> bool:
            if isinstance(value, dict):
                if value.get("tag") in wanted:
                    return True
                return any(contains_tag(child, wanted) for child in value.values())
            if isinstance(value, list):
                return any(contains_tag(child, wanted) for child in value)
            return False

        if contains_tag(elements, {"table", "chart", "form"}):
            self.error("form_children", f"{path}.elements", "form 内不支持 table、chart 或嵌套 form")
        if not any(
            isinstance(item, dict)
            and item.get("tag") == "button"
            and item.get("form_action_type") == "submit"
            for item in self._iter_dicts(elements)
        ):
            self.error("form_submit", f"{path}.elements", "form 内必须包含 form_action_type=submit 的按钮")

    def _lint_img(self, node: dict[str, Any], path: str) -> None:
        img_key = node.get("img_key")
        if not isinstance(img_key, str) or not img_key:
            self.error("img_key", f"{path}.img_key", "img 必须提供上传后得到的 img_key")
        elif img_key.startswith(("http://", "https://")):
            self.error("img_key_url", f"{path}.img_key", "img_key 不能是图片 URL，需先上传图片")
        elif self._looks_like_local_path(img_key):
            self._lint_local_image(img_key, f"{path}.img_key")
        elif not IMG_KEY_RE.fullmatch(img_key):
            self.error(
                "img_key_format",
                f"{path}.img_key",
                "必须是 img_ 开头的真实图片 key，或本地图片路径配合 --upload-images",
            )
        alt = node.get("alt")
        if (
            not isinstance(alt, dict)
            or alt.get("tag") != "plain_text"
            or not isinstance(alt.get("content"), str)
            or not alt.get("content", "").strip()
        ):
            self.error("img_alt", f"{path}.alt", "img 必须提供非空 plain_text alt")

    def _lint_img_combination(self, node: dict[str, Any], path: str) -> None:
        expected = {"double": 2, "triple": 3, "bisect": 4, "trisect": 6, "nine": 9}
        mode = node.get("combination_mode")
        if mode not in expected:
            self.error(
                "img_combination_mode",
                f"{path}.combination_mode",
                f"仅支持 {', '.join(expected)}",
            )
        images = node.get("img_list")
        if not isinstance(images, list):
            self.error("img_combination_list", f"{path}.img_list", "必须提供 img_list 数组")
            return
        if mode in expected and len(images) != expected[mode]:
            self.error(
                "img_combination_count",
                f"{path}.img_list",
                f"{mode} 模式必须正好提供 {expected[mode]} 张图片",
            )
        for index, image in enumerate(images):
            image_path = f"{path}.img_list[{index}].img_key"
            img_key = image.get("img_key") if isinstance(image, dict) else None
            if not isinstance(img_key, str) or not img_key:
                self.error("img_combination_key", image_path, "每张图片必须提供真实 img_key")
            elif img_key.startswith(("http://", "https://")):
                self.error(
                    "img_combination_key",
                    image_path,
                    "img_key 不能是 URL，需先取得卡片图片 token",
                )
            elif self._looks_like_local_path(img_key):
                self._lint_local_image(img_key, image_path)
            elif not IMG_KEY_RE.fullmatch(img_key):
                self.error(
                    "img_combination_key",
                    image_path,
                    "必须是 img_ 开头的真实图片 key，或本地图片路径配合 --upload-images",
                )

    def _lint_audio(self, node: dict[str, Any], path: str) -> None:
        background = node.get("background_style")
        if not isinstance(background, str) or not background.strip():
            self.error(
                "audio_background_style",
                f"{path}.background_style",
                "audio 必须提供非空 background_style",
            )
        self._lint_media_file_key(node.get("file_key"), f"{path}.file_key", component="audio")

    def _lint_video(self, node: dict[str, Any], path: str) -> None:
        self._lint_media_file_key(node.get("file_key"), f"{path}.file_key", component="video")
        cover = node.get("cover")
        if not isinstance(cover, dict):
            self.error("video_cover", f"{path}.cover", "video 必须提供 cover 对象")
            return
        self._lint_remote_image_key(
            cover.get("img_key"),
            f"{path}.cover.img_key",
            component="video 封面",
        )

    def _lint_media_file_key(self, value: Any, path: str, *, component: str) -> None:
        if not isinstance(value, str) or not value.strip():
            self.error(
                f"{component}_file_key",
                path,
                f"{component} 必须提供上传媒体后得到的非空 file_key",
            )
            return
        if (
            value.startswith(("http://", "https://", ".", "/", "~"))
            or "/" in value
            or "\\" in value
            or Path(value).suffix.lower() in MEDIA_FILE_EXTENSIONS
        ):
            self.error(
                f"{component}_file_key",
                path,
                "file_key 不能是 URL 或本地路径；--upload-images 不上传音视频文件，请先取得真实 file_key",
            )

    def _lint_avatar(self, node: dict[str, Any], path: str) -> None:
        user_id = node.get("user_id")
        if not isinstance(user_id, str) or not user_id.strip():
            self.error("avatar_user_id", f"{path}.user_id", "avatar 必须提供真实 user_id")

        size = node.get("size")
        if size not in AVATAR_SIZES:
            self.error(
                "avatar_size",
                f"{path}.size",
                f"avatar.size 必须是 {', '.join(sorted(AVATAR_SIZES))} 之一",
            )
        if size == "custom":
            custom_sizes = [
                node.get(field)
                for field in ("custom_size", "custom_size2")
                if node.get(field) is not None
            ]
            if not custom_sizes or any(
                not isinstance(value, (int, float))
                or isinstance(value, bool)
                or value <= 0
                for value in custom_sizes
            ):
                self.error(
                    "avatar_custom_size",
                    path,
                    "size=custom 时至少提供一个大于 0 的 custom_size/custom_size2",
                )

    def _lint_fallback_text(self, node: dict[str, Any], path: str) -> None:
        self._lint_text_object(
            node.get("text"),
            f"{path}.text",
            allowed_tags={"plain_text", "lark_md"},
        )

    def _lint_text_object(
        self,
        value: Any,
        path: str,
        *,
        allowed_tags: set[str],
    ) -> None:
        if not isinstance(value, dict) or value.get("tag") not in allowed_tags:
            self.error(
                "text_object",
                path,
                f"必须是 tag 为 {'/'.join(sorted(allowed_tags))} 的文本对象",
            )
            return
        content = value.get("content")
        i18n = value.get("i18n")
        has_content = isinstance(content, str) and bool(content.strip())
        has_i18n = isinstance(i18n, dict) and any(
            isinstance(item, str) and bool(item.strip()) for item in i18n.values()
        )
        if not has_content and not has_i18n:
            self.error("text_content", path, "文本对象必须提供非空 content 或 i18n 文案")

    def _lint_select_menu(self, node: dict[str, Any], path: str) -> None:
        tag = node["tag"]
        options = node.get("options")
        static = tag in {"select_static", "multi_select_static"}
        if static and (not isinstance(options, list) or not options):
            self.error("select_options", f"{path}.options", f"{tag} 必须提供非空 options 数组")
            return
        option_values: set[str] = set()
        if options is not None:
            if not isinstance(options, list):
                self.error("select_options", f"{path}.options", "options 必须是数组")
                return
            option_values = self._lint_select_options(options, f"{path}.options")

        select_type = node.get("type")
        if select_type is not None and select_type not in SELECT_TYPES:
            self.error(
                "select_type",
                f"{path}.type",
                f"仅支持 {', '.join(sorted(SELECT_TYPES))}",
            )

        initial = node.get("initial_option")
        if initial is not None and (
            not isinstance(initial, str)
            or not initial
            or (option_values and initial not in option_values)
        ):
            self.error(
                "select_initial",
                f"{path}.initial_option",
                "initial_option 必须引用 options 中存在的非空 value",
            )

        initial_index = node.get("initial_index")
        if initial_index is not None and (
            not isinstance(initial_index, int)
            or isinstance(initial_index, bool)
            or initial_index < 0
            or (isinstance(options, list) and initial_index >= len(options))
        ):
            self.error(
                "select_initial_index",
                f"{path}.initial_index",
                "initial_index 必须是 options 范围内的非负整数",
            )

        if tag in {"multi_select_static", "multi_select_person"}:
            self._lint_selected_values(
                node.get("selected_values"),
                f"{path}.selected_values",
                allowed=option_values,
            )

        if tag == "select_person":
            initial_user = node.get("initial_user")
            if initial_user is not None and (
                not isinstance(initial_user, str) or not initial_user.strip()
            ):
                self.error(
                    "select_initial_user",
                    f"{path}.initial_user",
                    "initial_user 必须是非空用户 ID",
                )

    def _lint_select_options(self, options: list[Any], path: str) -> set[str]:
        values: set[str] = set()
        for index, option in enumerate(options):
            option_path = f"{path}[{index}]"
            if not isinstance(option, dict):
                self.error("select_option", option_path, "每个选项必须是对象")
                continue
            self._lint_text_object(
                option.get("text"),
                f"{option_path}.text",
                allowed_tags={"plain_text"},
            )
            value = option.get("value")
            if not isinstance(value, str) or not value:
                self.error("select_option_value", f"{option_path}.value", "选项必须提供非空 value")
            elif value in values:
                self.error(
                    "select_option_duplicate",
                    f"{option_path}.value",
                    f"选项 value {value!r} 重复",
                )
            else:
                values.add(value)
        return values

    def _lint_selected_values(
        self,
        selected: Any,
        path: str,
        *,
        allowed: set[str],
    ) -> None:
        if selected is None:
            return
        if (
            not isinstance(selected, list)
            or any(not isinstance(value, str) or not value for value in selected)
        ):
            self.error("selected_values", path, "selected_values 必须是非空字符串数组")
            return
        if len(selected) != len(set(selected)):
            self.error("selected_values_duplicate", path, "selected_values 不得包含重复值")
        unknown = sorted(set(selected) - allowed) if allowed else []
        if unknown:
            self.error(
                "selected_values_unknown",
                path,
                f"默认值未在 options 中声明：{', '.join(unknown)}",
            )

    def _lint_select_img(self, node: dict[str, Any], path: str) -> None:
        style = node.get("style")
        if style not in SELECT_IMG_STYLES:
            self.error(
                "select_img_style",
                f"{path}.style",
                f"select_img.style 必须是 {', '.join(sorted(SELECT_IMG_STYLES))} 之一",
            )
        layout = node.get("layout")
        if layout is not None and layout not in SELECT_IMG_LAYOUTS:
            self.error(
                "select_img_layout",
                f"{path}.layout",
                f"仅支持 {', '.join(sorted(SELECT_IMG_LAYOUTS))}",
            )
        aspect_ratio = node.get("aspect_ratio")
        if aspect_ratio is not None and aspect_ratio not in SELECT_IMG_ASPECT_RATIOS:
            self.error(
                "select_img_aspect_ratio",
                f"{path}.aspect_ratio",
                f"仅支持 {', '.join(sorted(SELECT_IMG_ASPECT_RATIOS))}",
            )

        options = node.get("options")
        if not isinstance(options, list) or not options:
            self.error("select_img_options", f"{path}.options", "select_img 必须提供非空 options 数组")
            return
        values: set[str] = set()
        for index, option in enumerate(options):
            option_path = f"{path}.options[{index}]"
            if not isinstance(option, dict):
                self.error("select_img_option", option_path, "每个图片选项必须是对象")
                continue
            self._lint_remote_image_key(
                option.get("img_key"),
                f"{option_path}.img_key",
                component="select_img 选项",
            )
            value = option.get("value")
            if not isinstance(value, str) or not value:
                self.error(
                    "select_img_value",
                    f"{option_path}.value",
                    "每个图片选项必须提供非空 value",
                )
            elif value in values:
                self.error(
                    "select_img_value_duplicate",
                    f"{option_path}.value",
                    f"图片选项 value {value!r} 重复",
                )
            else:
                values.add(value)

        selected = node.get("selected_values")
        self._lint_selected_values(
            selected,
            f"{path}.selected_values",
            allowed=values,
        )
        if (
            node.get("multi_select") is not True
            and isinstance(selected, list)
            and len(selected) > 1
        ):
            self.error(
                "select_img_single",
                f"{path}.selected_values",
                "multi_select 未开启时最多只能有一个默认选中值",
            )

    def _lint_remote_image_key(self, value: Any, path: str, *, component: str) -> None:
        if not isinstance(value, str) or not value:
            self.error("image_key", path, f"{component} 必须提供真实 img_key")
            return
        if value.startswith(("http://", "https://")):
            self.error("image_key_url", path, f"{component} 的 img_key 不能是图片 URL")
        elif self._looks_like_local_path(value):
            self.error(
                "image_key_upload_scope",
                path,
                f"{component} 暂不支持通过 --upload-images 替换本地路径；请先上传并填入真实 img_key",
            )
        elif not IMG_KEY_RE.fullmatch(value):
            self.error("image_key_format", path, f"{component} 必须使用 img_ 开头的真实图片 key")

    def _lint_date_picker(self, node: dict[str, Any], path: str) -> None:
        tag = node["tag"]
        expected = {
            "date_picker": ("initial_date", DATE_RE, "YYYY-MM-DD"),
            "picker_date": ("initial_date", DATE_RE, "YYYY-MM-DD"),
            "picker_time": ("initial_time", TIME_RE, "HH:mm"),
            "picker_datetime": (
                "initial_datetime",
                DATETIME_RE,
                "YYYY-MM-DD HH:mm",
            ),
        }
        field, pattern, format_hint = expected[tag]
        incompatible = {
            "initial_date",
            "initial_time",
            "initial_datetime",
        } - {field}
        for other in sorted(incompatible & node.keys()):
            self.error(
                "picker_initial_field",
                f"{path}.{other}",
                f"{tag} 应使用 {field}，不要使用 {other}",
            )
        value = node.get(field)
        if value is not None and (
            not isinstance(value, str) or not pattern.fullmatch(value)
        ):
            self.error(
                "picker_initial_format",
                f"{path}.{field}",
                f"{tag}.{field} 必须使用 {format_hint} 格式",
            )

    def _lint_icon(self, node: dict[str, Any], path: str) -> None:
        tag = node["tag"]
        if tag == "custom_icon":
            self._lint_remote_image_key(
                node.get("img_key"),
                f"{path}.img_key",
                component="custom_icon",
            )
            return
        token = node.get("token")
        if not isinstance(token, str) or not token.strip():
            self.error("icon_token", f"{path}.token", f"{tag} 必须提供非空 token")
        if tag == "ud_icon":
            style = node.get("style")
            if not isinstance(style, str) or not style.strip():
                self.error("ud_icon_style", f"{path}.style", "ud_icon 必须提供非空 style")

    def _lint_local_image(self, target: str, path: str) -> None:
        self.local_image_paths.add(target)
        try:
            resolved = Path(target).expanduser()
        except RuntimeError:
            self.error(
                "local_image_missing",
                path,
                f"无法解析本地图片用户目录：{target!r}",
            )
            return
        if not resolved.is_absolute():
            resolved = self.base_dir / resolved
        if not resolved.is_file():
            self.error(
                "local_image_missing",
                path,
                f"本地图片不存在或不是文件：{target!r}",
            )
        elif not self.upload_images:
            self.warning(
                "local_image_upload",
                path,
                f"本地图片 {target!r} 发送时必须加 --upload-images；lint 时也传同名 flag",
            )

    def _lint_person_list(self, node: dict[str, Any], path: str) -> None:
        persons = node.get("persons")
        if not isinstance(persons, list) or not persons:
            self.error("person_list", f"{path}.persons", "person_list 必须提供非空 persons 数组")
            return
        for index, person in enumerate(persons):
            if not isinstance(person, dict) or not isinstance(person.get("id"), str) or not person.get("id"):
                self.error(
                    "person_list_id",
                    f"{path}.persons[{index}].id",
                    "每个人员项必须使用真实 ID；字段名是 id，不是 user_id",
                )
            elif "user_id" in person:
                self.error(
                    "person_list_user_id",
                    f"{path}.persons[{index}].user_id",
                    "person_list 人员项不支持 user_id；只保留 id",
                )

    def _lint_button(self, node: dict[str, Any], path: str, in_form: bool) -> None:
        behaviors = node.get("behaviors")
        form_action = node.get("form_action_type")
        has_form_action = in_form and form_action in {"submit", "reset"}
        if form_action is not None and not in_form:
            self.error(
                "form_action_scope",
                f"{path}.form_action_type",
                "form_action_type 只能用于 form 内的按钮",
            )
        has_legacy_action = node.get("action_type") in {
            "link",
            "request",
            "multi",
            "form_submit",
            "form_reset",
        }
        if not behaviors and not has_form_action and not has_legacy_action:
            self.warning(
                "button_action",
                path,
                "按钮没有 behaviors 或表单动作；没有真实行动入口时应删除按钮",
            )
            return
        if node.get("action_type") == "link":
            legacy_url = node.get("url")
            multi_url = node.get("multi_url")
            if isinstance(legacy_url, str):
                self._lint_url_fields({"url": legacy_url}, path, require_default=True)
            elif isinstance(multi_url, dict):
                self._lint_url_fields(multi_url, f"{path}.multi_url", require_default=True)
            else:
                self.error("legacy_link_url", path, "action_type=link 必须提供真实链接")
        if behaviors is not None and not isinstance(behaviors, list):
            self.error("behaviors_type", f"{path}.behaviors", "behaviors 必须是数组")
        elif isinstance(behaviors, list):
            self._lint_behaviors(behaviors, f"{path}.behaviors")

    def _lint_interactive_container(self, node: dict[str, Any], path: str) -> None:
        behaviors = node.get("behaviors")
        if not isinstance(behaviors, list) or not behaviors:
            if node.get("hover_tips") is not None:
                self.warning(
                    "interactive_action",
                    path,
                    "interactive_container 显示 hover_tips 却没有 behaviors，容易形成假交互",
                )
            return
        self._lint_behaviors(behaviors, f"{path}.behaviors")

    def _lint_behaviors(self, behaviors: list[Any], path: str) -> None:
        for index, behavior in enumerate(behaviors):
            behavior_path = f"{path}[{index}]"
            if not isinstance(behavior, dict):
                self.error("behavior_type", behavior_path, "behavior 必须是对象")
                continue
            behavior_type = behavior.get("type")
            if behavior_type == "open_url":
                self._lint_url_fields(behavior, behavior_path, require_default=True)
            elif behavior_type == "callback":
                value = behavior.get("value")
                if not isinstance(value, dict) or not value:
                    self.warning(
                        "callback_value",
                        f"{behavior_path}.value",
                        "callback 建议提供可识别的非空 value 对象",
                    )
            elif behavior_type not in {"event", "client_message", "template_open_url"}:
                self.warning(
                    "behavior_unknown",
                    f"{behavior_path}.type",
                    f"未收录 behavior 类型 {behavior_type!r}，请核对官方文档",
                )

    def _lint_url_fields(
        self,
        value: dict[str, Any],
        path: str,
        *,
        require_default: bool,
    ) -> None:
        default_key = "default_url" if "type" in value else "url"
        url_keys = (default_key, "pc_url", "ios_url", "android_url")
        if require_default and not value.get(default_key):
            self.error("open_url", f"{path}.{default_key}", f"必须提供真实 {default_key}")
        for key in url_keys:
            if key not in value:
                continue
            url = value.get(key)
            if (
                self.allow_placeholders
                and isinstance(url, str)
                and PLACEHOLDER_RE.search(url)
            ):
                continue
            if not isinstance(url, str) or not self._valid_url(url):
                self.error("invalid_url", f"{path}.{key}", "必须是带协议的真实 URL")
            elif url.startswith("http://"):
                self.warning("insecure_url", f"{path}.{key}", "优先使用 HTTPS")

    @staticmethod
    def _valid_url(value: str) -> bool:
        parsed = urlparse(value)
        if not parsed.scheme:
            return False
        if parsed.scheme in {"http", "https"}:
            return bool(parsed.netloc)
        return parsed.scheme in {"lark", "mailto"}

    @staticmethod
    def _looks_like_local_path(value: str) -> bool:
        return (
            value.startswith((".", "/", "~"))
            or "/" in value
            or "\\" in value
            or Path(value).suffix.lower()
            in {".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff"}
        )

    def _lint_input(self, node: dict[str, Any], path: str) -> None:
        input_type = node.get("input_type", "text")
        if input_type not in {"text", "multiline_text", "password"}:
            self.error(
                "input_type",
                f"{path}.input_type",
                "仅支持 text、multiline_text、password",
            )
        multiline_fields = {"rows", "auto_resize", "max_rows"} & node.keys()
        if multiline_fields and input_type != "multiline_text":
            self.error(
                "input_multiline",
                path,
                f"{', '.join(sorted(multiline_fields))} 只在 input_type=multiline_text 时有效",
            )

    def _lint_chart(self, node: dict[str, Any], path: str) -> None:
        spec = node.get("chart_spec")
        if not isinstance(spec, dict):
            self.error("chart_spec", f"{path}.chart_spec", "chart 必须提供 chart_spec 对象")
            return
        values = spec.get("data", {}).get("values") if isinstance(spec.get("data"), dict) else None
        if not isinstance(values, list) or not values or not isinstance(values[0], dict):
            return
        keys = set(values[0])
        for field_name in ("xField", "yField", "categoryField", "valueField", "seriesField"):
            field = spec.get(field_name)
            field_values = field if isinstance(field, list) else [field]
            for field_value in field_values:
                if isinstance(field_value, str) and field_value not in keys:
                    self.error(
                        "chart_field",
                        f"{path}.chart_spec.{field_name}",
                        f"字段 {field_value!r} 不在 data.values 首行中",
                    )

    def _scan_values(self, node: Any, path: str = "$") -> None:
        if isinstance(node, dict):
            for key, value in node.items():
                if PLACEHOLDER_RE.search(key):
                    if self.allow_placeholders:
                        self.warning(
                            "draft_placeholder",
                            f"{path}.{key}",
                            f"草稿仍有模板标记键：{key}",
                        )
                    else:
                        self.error(
                            "placeholder",
                            f"{path}.{key}",
                            f"存在未移除的模板标记键：{key}",
                        )
                self._scan_values(value, f"{path}.{key}")
            return
        if isinstance(node, list):
            for index, value in enumerate(node):
                self._scan_values(value, f"{path}[{index}]")
            return
        if not isinstance(node, str):
            return

        matches = sorted(set(PLACEHOLDER_RE.findall(node)))
        if matches:
            if self.allow_placeholders:
                self.warning(
                    "draft_placeholder",
                    path,
                    f"草稿未决项：{', '.join(matches)}",
                )
            else:
                self.error("placeholder", path, f"存在未替换占位符：{', '.join(matches)}")
        if GENERIC_ID_RE.fullmatch(node) or node in {"https://...", "http://..."}:
            if self.allow_placeholders:
                self.warning("draft_example_id", path, f"草稿仍有示例标识 {node!r}")
            else:
                self.error("placeholder", path, f"存在示例标识 {node!r}")
        if "example.com" in node:
            if self.allow_placeholders:
                self.warning("draft_example_url", path, "草稿仍有 example.com 示例 URL")
            else:
                self.error("example_url", path, "示例 URL 不可进入发送候选；补真实链接或删除行动入口")

        if path.endswith(".content"):
            for target in MARKDOWN_IMAGE_RE.findall(node):
                if target.startswith(("http://", "https://")):
                    self.error(
                        "markdown_image_url",
                        path,
                        f"Markdown 图片 {target!r} 不能直接使用 URL；请上传或改用本地路径配合 --upload-images",
                    )
                elif not target.startswith(("img_", "file_")):
                    self._lint_local_image(target, path)

    @staticmethod
    def _iter_dicts(value: Any):
        if isinstance(value, dict):
            yield value
            for child in value.values():
                yield from CardLinter._iter_dicts(child)
        elif isinstance(value, list):
            for child in value:
                yield from CardLinter._iter_dicts(child)


def load_json(path: Path | None) -> tuple[Any, int]:
    raw = path.read_bytes() if path and str(path) != "-" else sys.stdin.buffer.read()
    try:
        return json.loads(raw), len(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ValueError(f"不是有效的 UTF-8 JSON：{exc}") from exc


def render_text(issues: list[Issue], *, strict: bool) -> int:
    for issue in issues:
        label = "错误" if issue.level == "error" else "警告"
        stream = sys.stderr if issue.level == "error" or strict else sys.stdout
        print(f"{label} [{issue.code}] {issue.path}: {issue.message}", file=stream)
    errors = sum(issue.level == "error" for issue in issues)
    warnings = sum(issue.level == "warning" for issue in issues)
    if errors == 0 and (not strict or warnings == 0):
        print(f"通过：Card JSON 2.0 离线检查（{warnings} 条警告）")
        return 0
    return 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="离线检查飞书 Card JSON 2.0")
    parser.add_argument("card", nargs="?", type=Path, help="卡片 JSON 文件；省略或传 - 时读取 stdin")
    parser.add_argument(
        "--allow-placeholders",
        action="store_true",
        help="允许草稿中的 TODO/占位符，但仍作为 warning 列出",
    )
    parser.add_argument(
        "--upload-images",
        action="store_true",
        help="确认发送时会使用 msg send --upload-images，并检查本地图片存在",
    )
    parser.add_argument("--strict", action="store_true", help="把 warning 也视为失败")
    parser.add_argument("--json", action="store_true", dest="json_output", help="输出机器可读 JSON")
    args = parser.parse_args(argv)

    try:
        data, raw_size = load_json(args.card)
    except (OSError, ValueError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return 2

    base_dir = args.card.parent if args.card and str(args.card) != "-" else Path.cwd()
    linter = CardLinter(
        allow_placeholders=args.allow_placeholders,
        upload_images=args.upload_images,
        base_dir=base_dir,
    )
    issues = linter.lint(data, raw_size)
    errors = sum(issue.level == "error" for issue in issues)
    warnings = sum(issue.level == "warning" for issue in issues)
    valid = errors == 0 and (not args.strict or warnings == 0)

    if args.json_output:
        print(
            json.dumps(
                {
                    "valid": valid,
                    "errors": errors,
                    "warnings": warnings,
                    "local_images": sorted(linter.local_image_paths),
                    "issues": [asdict(issue) for issue in issues],
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 0 if valid else 1

    return render_text(issues, strict=args.strict)


if __name__ == "__main__":
    raise SystemExit(main())
