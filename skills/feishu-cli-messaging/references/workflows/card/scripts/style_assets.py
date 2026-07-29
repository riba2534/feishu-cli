#!/usr/bin/env python3
"""选择卡片预设风格、检索/复制素材并生成可校验的 Card JSON 2.0 起稿。"""

from __future__ import annotations

import argparse
import difflib
import json
import re
import shutil
import sys
from pathlib import Path
from typing import Any


CARD_ROOT = Path(__file__).resolve().parent.parent
ASSETS_ROOT = CARD_ROOT / "assets"
DOODLE_ROOT = ASSETS_ROOT / "icons" / "doodle"
PHOSPHOR_ROOT = ASSETS_ROOT / "icons" / "phosphor"
HEADER_ROOT = ASSETS_ROOT / "headers" / "modern-minimal"
PALETTE_FILE = ASSETS_ROOT / "themes" / "custom-visual-palettes.json"
MODERN_TOKEN_FILE = ASSETS_ROOT / "themes" / "modern-minimal-tokens.json"

DASHBOARD_PRESETS = [
    {
        "style_id": "dashboard-monitoring",
        "name": "监控态势型",
        "family": "dashboard",
        "vibe": "状态、时间窗口、核心异常",
        "header_template": "blue",
        "layout": "monitoring",
    },
    {
        "style_id": "dashboard-business",
        "name": "经营汇报型",
        "family": "dashboard",
        "vibe": "一句业务结论、KPI 达成、主要变化",
        "header_template": "indigo",
        "layout": "business",
    },
    {
        "style_id": "dashboard-trend",
        "name": "趋势分析型",
        "family": "dashboard",
        "vibe": "趋势方向、拐点与原因",
        "header_template": "purple",
        "layout": "trend",
    },
    {
        "style_id": "dashboard-ranking",
        "name": "排行洞察型",
        "family": "dashboard",
        "vibe": "榜首、末尾、最大变化与异常项",
        "header_template": "purple",
        "layout": "ranking",
    },
    {
        "style_id": "dashboard-health",
        "name": "健康评分型",
        "family": "dashboard",
        "vibe": "评分、等级与主要扣分项",
        "header_template": "green",
        "layout": "health",
    },
    {
        "style_id": "dashboard-minimal",
        "name": "极简概览型",
        "family": "dashboard",
        "vibe": "一句结论、3–4 个指标和一个风险或下一步",
        "header_template": "blue",
        "layout": "minimal",
    },
]

BASE_STYLES = [
    {
        "style_id": "default-semantic",
        "name": "默认语义",
        "family": "semantic",
        "vibe": "用 header 颜色表达信息、成功、注意、故障或归档状态",
    },
    {
        "style_id": "modern-minimal",
        "name": "Modern Minimal",
        "family": "modern-minimal",
        "vibe": "克制、留白、冷静的 SaaS / API / 企业平台视觉",
    },
]

ALIASES = {
    "semantic": "default-semantic",
    "default": "default-semantic",
    "dashboard": "dashboard-minimal",
    "monitoring": "dashboard-monitoring",
    "business": "dashboard-business",
    "trend": "dashboard-trend",
    "ranking": "dashboard-ranking",
    "health": "dashboard-health",
    "minimal-dashboard": "dashboard-minimal",
    "modern": "modern-minimal",
    "minimal": "modern-minimal",
}

DEFAULT_CUSTOM_ICONS = {
    "handdrawn-vibes": "doodle:magic-wand.safe16",
    "handdrawn-dark": "doodle:magic-wand.safe16",
    "wellness-planner": "doodle:heart.safe16",
    "claude-chrome": "doodle:puzzle.safe16",
}

DEFAULT_MODERN_ICONS = {
    "info": "phosphor:phosphor_regular_info_status-feedback",
    "success": "phosphor:phosphor_regular_check-circle_status-feedback",
    "warning": "phosphor:phosphor_regular_warning_safety-emergency",
    "danger": "phosphor:phosphor_regular_seal-warning_safety-emergency",
    "neutral": "phosphor:phosphor_regular_sparkle_ai-technology",
}

COLOR_RE = re.compile(
    r"rgba?\(\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,"
    r"\s*(\d+(?:\.\d+)?)\s*(?:,\s*(\d*\.?\d+)\s*)?\)"
)


def load_json(path: Path) -> Any:
    with path.open(encoding="utf-8") as handle:
        return json.load(handle)


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with path.open(encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, 1):
            if not line.strip():
                continue
            try:
                rows.append(json.loads(line))
            except json.JSONDecodeError as exc:
                raise ValueError(f"{path}:{line_number} 不是有效 JSONL：{exc}") from exc
    return rows


def custom_styles() -> list[dict[str, Any]]:
    styles = []
    for item in load_json(PALETTE_FILE)["styles"]:
        row = dict(item)
        row["family"] = "custom-visual"
        row.setdefault("vibe", row.get("description", "自定义视觉主题"))
        row.setdefault("usage", [])
        styles.append(row)
    return styles


def style_catalog() -> list[dict[str, Any]]:
    return BASE_STYLES + DASHBOARD_PRESETS + custom_styles()


def normalize_style_id(value: str) -> str:
    normalized = re.sub(r"[\s_]+", "-", value.strip().lower())
    return ALIASES.get(normalized, normalized)


def get_style(style_id: str) -> dict[str, Any]:
    normalized = normalize_style_id(style_id)
    catalog = {item["style_id"]: item for item in style_catalog()}
    if normalized in catalog:
        return catalog[normalized]
    matches = difflib.get_close_matches(normalized, catalog, n=3)
    hint = f"；你是否想输入：{', '.join(matches)}" if matches else ""
    raise ValueError(f"未知 style_id：{style_id}{hint}")


def header_assets() -> list[dict[str, Any]]:
    manifest = load_json(HEADER_ROOT / "manifest.json")
    result = []
    for item in manifest["assets"]:
        row = dict(item)
        row["family"] = "header"
        row["asset_id"] = f"header:{item['id']}"
        row["path"] = str((HEADER_ROOT / item["local_file"]).resolve())
        row["tags"] = ["header", "modern-minimal", item["state"]]
        result.append(row)
    return result


def icon_assets(family: str, root: Path) -> list[dict[str, Any]]:
    result = []
    for item in load_jsonl(root / "manifest.jsonl"):
        row = dict(item)
        row["family"] = family
        row["asset_id"] = f"{family}:{item['filekey']}"
        row["path"] = str((root / f"{item['filekey']}.png").resolve())
        result.append(row)
    return result


def asset_catalog() -> list[dict[str, Any]]:
    return (
        header_assets()
        + icon_assets("doodle", DOODLE_ROOT)
        + icon_assets("phosphor", PHOSPHOR_ROOT)
    )


def get_asset(asset_id: str) -> dict[str, Any]:
    catalog = asset_catalog()
    exact = [item for item in catalog if item["asset_id"] == asset_id]
    if len(exact) == 1:
        return exact[0]
    if ":" not in asset_id:
        bare = [
            item
            for item in catalog
            if item["asset_id"].split(":", 1)[1] == asset_id
            or Path(item["path"]).name == asset_id
        ]
        if len(bare) == 1:
            return bare[0]
        if len(bare) > 1:
            raise ValueError(
                f"素材名 {asset_id!r} 不唯一，请使用带 family 的 ID："
                + ", ".join(item["asset_id"] for item in bare)
            )
    choices = [item["asset_id"] for item in catalog]
    matches = difflib.get_close_matches(asset_id, choices, n=5)
    hint = f"；相近素材：{', '.join(matches)}" if matches else ""
    raise ValueError(f"找不到素材 {asset_id!r}{hint}")


def parse_color(value: str) -> tuple[float, float, float, float]:
    if re.fullmatch(r"#[0-9A-Fa-f]{6}", value):
        return tuple(int(value[index : index + 2], 16) / 255 for index in (1, 3, 5)) + (1.0,)
    match = COLOR_RE.fullmatch(value)
    if not match:
        raise ValueError(f"不支持的颜色格式：{value}")
    red, green, blue = (float(match.group(index)) / 255 for index in (1, 2, 3))
    alpha = float(match.group(4)) if match.group(4) is not None else 1.0
    return red, green, blue, alpha


def rgba_string(value: str) -> str:
    red, green, blue, alpha = parse_color(value)
    alpha_text = f"{alpha:.3f}".rstrip("0").rstrip(".")
    return (
        f"rgba({round(red * 255)}, {round(green * 255)}, "
        f"{round(blue * 255)}, {alpha_text})"
    )


def composite(rgb: tuple[float, float, float, float], background: tuple[float, ...]) -> tuple[float, ...]:
    red, green, blue, alpha = rgb
    return tuple(
        channel * alpha + background[index] * (1 - alpha)
        for index, channel in enumerate((red, green, blue))
    )


def luminance(rgb: tuple[float, ...]) -> float:
    def channel(value: float) -> float:
        return value / 12.92 if value <= 0.04045 else ((value + 0.055) / 1.055) ** 2.4

    red, green, blue = (channel(value) for value in rgb[:3])
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue


def contrast(first: tuple[float, ...], second: tuple[float, ...]) -> float:
    lighter, darker = sorted((luminance(first), luminance(second)), reverse=True)
    return (lighter + 0.05) / (darker + 0.05)


def token_prefix(style_id: str) -> str:
    return "wb_" + re.sub(r"[^a-z0-9]+", "_", style_id.lower()).strip("_")


def choose_foreground(background: str, palette: dict[str, str]) -> str:
    canvas = parse_color(palette.get("canvas", "#FFFFFF"))
    bg = composite(parse_color(background), canvas)
    candidates = [
        palette.get("ink", "#111111"),
        palette.get("button_text", "#FFFFFF"),
        "#000000",
        "#FFFFFF",
    ]
    return max(candidates, key=lambda candidate: contrast(bg, parse_color(candidate)))


def contrast_on(background: str, foreground: str, *, base: str = "#FFFFFF") -> float:
    base_rgb = parse_color(base)
    background_rgb = composite(parse_color(background), base_rgb)
    foreground_rgb = composite(parse_color(foreground), background_rgb)
    return contrast(background_rgb, foreground_rgb)


def readable_text(preferred: str, background: str, palette: dict[str, str]) -> str:
    if contrast_on(background, preferred) >= 4.5:
        return preferred
    return choose_foreground(background, palette)


def custom_theme_tokens(style: dict[str, Any]) -> dict[str, Any]:
    prefix = token_prefix(style["style_id"])
    palette: dict[str, str] = style["colors"]
    tokens: dict[str, Any] = {}
    for role, color in palette.items():
        tokens[f"{prefix}_{role}"] = {
            "light_mode": rgba_string(color),
            "dark_mode": rgba_string(color),
        }
    for role, color in palette.items():
        foreground = choose_foreground(color, palette)
        tokens[f"{prefix}_on_{role}"] = {
            "light_mode": rgba_string(foreground),
            "dark_mode": rgba_string(foreground),
        }
    muted_text = readable_text(
        palette.get("muted", palette["ink"]),
        palette["canvas"],
        palette,
    )
    tokens[f"{prefix}_muted_text"] = {
        "light_mode": rgba_string(muted_text),
        "dark_mode": rgba_string(muted_text),
    }
    return tokens


def style_tokens(style_id: str) -> dict[str, Any]:
    style = get_style(style_id)
    if style["family"] == "custom-visual":
        colors = custom_theme_tokens(style)
        prefix = token_prefix(style["style_id"])
    elif style["style_id"] == "modern-minimal":
        colors = load_json(MODERN_TOKEN_FILE)["colors"]
        prefix = "mm"
    else:
        colors = {}
        prefix = ""
    return {
        "schema_version": "1.0",
        "style_id": style["style_id"],
        "token_prefix": prefix,
        "config": {"style": {"color": colors}} if colors else {},
    }


def write_json(path: Path | None, data: Any, *, force: bool = False) -> None:
    rendered = json.dumps(data, ensure_ascii=False, indent=2) + "\n"
    if path is None:
        sys.stdout.write(rendered)
        return
    if (path.exists() or path.is_symlink()) and not force:
        raise ValueError(f"输出已存在：{path}；确认覆盖时加 --force")
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.is_symlink():
        path.unlink()
    path.write_text(rendered, encoding="utf-8")
    print(path.resolve())


def plain(content: str) -> dict[str, str]:
    return {"tag": "plain_text", "content": content}


def markdown(content: str, **extra: Any) -> dict[str, Any]:
    result: dict[str, Any] = {"tag": "markdown", "content": content, "text_align": "left"}
    result.update(extra)
    return result


def custom_starter(style: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    prefix = token_prefix(style["style_id"])
    palette = style["colors"]
    icon_id = args.icon or DEFAULT_CUSTOM_ICONS.get(
        style["style_id"], "doodle:bulb.safe16"
    )
    icon = get_asset(icon_id)
    if icon["family"] != "doodle":
        raise ValueError(
            f"{style['style_id']} 只接受 doodle 素材；实际为 {icon['asset_id']}"
        )
    icon_path = icon["path"]
    accent_2 = "accent_2" if "accent_2" in palette else "accent"
    icon_surface = "icon_surface" if "icon_surface" in palette else "panel"
    title = args.title or "TODO_TITLE"
    summary = args.summary or "TODO_SUMMARY"
    detail = args.detail or "TODO_DETAIL"
    meta = args.meta or style["name"]

    return {
        "schema": "2.0",
        "config": {
            "update_multi": True,
            "summary": {"content": title},
            "style": {"color": custom_theme_tokens(style)},
        },
        "body": {
            "direction": "vertical",
            "vertical_spacing": "8px",
            "padding": "0px 0px 0px 0px",
            "elements": [
                {
                    "tag": "interactive_container",
                    "width": "fill",
                    "height": "auto",
                    "corner_radius": "0px",
                    "has_border": False,
                    "disabled": False,
                    "background_style": f"{prefix}_canvas",
                    "padding": "20px 20px 20px 20px",
                    "direction": "vertical",
                    "vertical_spacing": "8px",
                    "horizontal_align": "left",
                    "vertical_align": "top",
                    "elements": [
                        {
                            "tag": "column_set",
                            "flex_mode": "none",
                            "background_style": "default",
                            "horizontal_spacing": "8px",
                            "columns": [
                                {
                                    "tag": "column",
                                    "width": "auto",
                                    "weight": 1,
                                    "elements": [
                                        {
                                            "tag": "interactive_container",
                                            "width": "auto",
                                            "height": "auto",
                                            "corner_radius": "999px",
                                            "has_border": False,
                                            "disabled": False,
                                            "background_style": f"{prefix}_panel",
                                            "padding": "3px 6px 3px 6px",
                                            "direction": "vertical",
                                            "horizontal_align": "center",
                                            "elements": [
                                                markdown(
                                                    f"<font color='{prefix}_on_panel'>{meta}</font>",
                                                    text_size="notation",
                                                    text_align="center",
                                                )
                                            ],
                                        }
                                    ],
                                }
                            ],
                        },
                        markdown(
                            f"**<font color='{prefix}_ink'>{title}</font>**",
                            text_size="heading",
                        ),
                        markdown(
                            f"<font color='{prefix}_muted_text'>{summary}</font>",
                            text_size="normal",
                        ),
                        {
                            "tag": "column_set",
                            "flex_mode": "stretch",
                            "background_style": "default",
                            "horizontal_spacing": "8px",
                            "columns": [
                                {
                                    "tag": "column",
                                    "width": "weighted",
                                    "weight": 1,
                                    "background_style": f"{prefix}_accent",
                                    "padding": "14px 14px 14px 14px",
                                    "vertical_spacing": "8px",
                                    "elements": [
                                        {
                                            "tag": "interactive_container",
                                            "width": "48px",
                                            "height": "48px",
                                            "corner_radius": "12px",
                                            "has_border": False,
                                            "disabled": False,
                                            "background_style": f"{prefix}_{icon_surface}",
                                            "padding": "8px 8px 8px 8px",
                                            "direction": "vertical",
                                            "horizontal_align": "center",
                                            "elements": [
                                                {
                                                    "tag": "img",
                                                    "img_key": icon_path,
                                                    "preview": False,
                                                    "transparent": True,
                                                    "scale_type": "crop_center",
                                                    "size": "32px 32px",
                                                    "alt": plain("主题语义图标"),
                                                }
                                            ],
                                        },
                                        markdown(
                                            f"**<font color='{prefix}_on_accent'>{summary}</font>**",
                                            text_size="normal",
                                        ),
                                    ],
                                },
                                {
                                    "tag": "column",
                                    "width": "weighted",
                                    "weight": 1,
                                    "background_style": f"{prefix}_{accent_2}",
                                    "padding": "14px 14px 14px 14px",
                                    "vertical_spacing": "8px",
                                    "elements": [
                                        markdown(
                                            f"<font color='{prefix}_on_{accent_2}'>{detail}</font>",
                                            text_size="normal",
                                        )
                                    ],
                                },
                            ],
                        },
                        markdown(
                            f"<font color='{prefix}_muted_text'>视觉预设：{style['name']}</font>",
                            text_size="notation",
                        ),
                    ],
                }
            ],
        },
    }


def modern_starter(style: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    state = args.header_state
    header = next(item for item in header_assets() if item["state"] == state)
    icon = get_asset(args.icon or DEFAULT_MODERN_ICONS[state])
    if icon["family"] != "phosphor":
        raise ValueError(
            f"modern-minimal 只接受 phosphor 素材；实际为 {icon['asset_id']}"
        )
    icon_path = icon["path"]
    title = args.title or "TODO_TITLE"
    summary = args.summary or "TODO_SUMMARY"
    detail = args.detail or "TODO_DETAIL"
    meta = args.meta or "Modern Minimal"
    tokens = load_json(MODERN_TOKEN_FILE)["colors"]

    return {
        "schema": "2.0",
        "config": {
            "update_multi": True,
            "summary": {"content": title},
            "style": {"color": tokens},
        },
        "body": {
            "direction": "vertical",
            "horizontal_spacing": "8px",
            "vertical_spacing": "8px",
            "horizontal_align": "left",
            "vertical_align": "top",
            "padding": "0px 0px 0px 0px",
            "elements": [
                {
                    "tag": "img",
                    "img_key": header["path"],
                    "alt": plain(f"{state} 状态封面"),
                    "preview": False,
                    "transparent": True,
                    "scale_type": "fit_horizontal",
                    "margin": "0px 0px 0px 0px",
                },
                markdown(
                    f"**<font color='indigo'>{title}</font>**",
                    text_size="heading",
                    margin="-24px 20px 0px 20px",
                ),
                markdown(
                    f"<font color='grey'>{meta}</font>",
                    text_size="normal",
                    margin="-8px 20px 0px 20px",
                ),
                {
                    "tag": "interactive_container",
                    "width": "fill",
                    "height": "auto",
                    "corner_radius": "10px",
                    "has_border": False,
                    "disabled": False,
                    "background_style": "mm_key_point_blue_surface",
                    "padding": "14px 16px 14px 16px",
                    "direction": "vertical",
                    "vertical_spacing": "8px",
                    "margin": "8px 20px 0px 20px",
                    "elements": [
                        {
                            "tag": "column_set",
                            "flex_mode": "stretch",
                            "background_style": "default",
                            "horizontal_spacing": "12px",
                            "columns": [
                                {
                                    "tag": "column",
                                    "width": "auto",
                                    "weight": 1,
                                    "elements": [
                                        {
                                            "tag": "interactive_container",
                                            "width": "48px",
                                            "height": "48px",
                                            "corner_radius": "12px",
                                            "has_border": True,
                                            "border_color": "mm_key_point_icon_border",
                                            "background_style": "mm_key_point_icon_surface",
                                            "padding": "12px 12px 12px 12px",
                                            "direction": "vertical",
                                            "horizontal_align": "center",
                                            "disabled": False,
                                            "elements": [
                                                {
                                                    "tag": "img",
                                                    "img_key": icon_path,
                                                    "preview": False,
                                                    "transparent": True,
                                                    "scale_type": "crop_center",
                                                    "size": "24px 24px",
                                                    "alt": plain("摘要语义图标"),
                                                }
                                            ],
                                        }
                                    ],
                                },
                                {
                                    "tag": "column",
                                    "width": "weighted",
                                    "weight": 1,
                                    "vertical_spacing": "4px",
                                    "elements": [
                                        markdown(
                                            f"**<font color='mm_key_point_ink'>{summary}</font>**",
                                            text_size="normal",
                                        ),
                                        markdown(
                                            f"<font color='mm_key_point_muted'>{detail}</font>",
                                            text_size="notation",
                                        ),
                                    ],
                                },
                            ],
                        }
                    ],
                },
                {
                    "tag": "interactive_container",
                    "width": "fill",
                    "height": "auto",
                    "corner_radius": "0px",
                    "has_border": False,
                    "disabled": False,
                    "background_style": "bottom_bg",
                    "padding": "8px 0px 8px 0px",
                    "direction": "vertical",
                    "elements": [
                        markdown(
                            "<font color='footer_text'>视觉预设：Modern Minimal</font>",
                            text_size="small",
                            margin="0px 20px 0px 16px",
                        )
                    ],
                },
            ],
        },
    }


def dashboard_starter(style: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    title = args.title or "TODO_TITLE"
    summary = args.summary or "TODO_SUMMARY"
    detail = args.detail or f"TODO_{style['layout'].upper()}_DETAIL"
    meta = args.meta or "TODO_TIME_RANGE_AND_SCOPE"
    metrics = args.metric or ["TODO_METRIC_A=TODO_VALUE_A", "TODO_METRIC_B=TODO_VALUE_B"]
    fields = []
    rows = []
    for item in metrics[:4]:
        if "=" not in item:
            raise ValueError(f"--metric 必须使用 标签=数值：{item!r}")
        label, value = item.split("=", 1)
        label = label.strip()
        value = value.strip()
        fields.append(
            {
                "is_short": True,
                "text": {
                    "tag": "lark_md",
                    "content": f"**{label}**\n{value}",
                },
            }
        )
        rows.append({"item": label, "value": value})

    conclusion = markdown(f"**结论**\n{summary}")
    metric_div = {"tag": "div", "fields": fields}
    layout = style["layout"]
    if layout == "monitoring":
        elements = [
            conclusion,
            metric_div,
            {
                "tag": "interactive_container",
                "width": "fill",
                "height": "auto",
                "corner_radius": "8px",
                "has_border": False,
                "disabled": False,
                "background_style": "orange-50",
                "padding": "10px 12px 10px 12px",
                "direction": "vertical",
                "elements": [markdown(f"**异常与处置**\n{detail}")],
            },
        ]
    elif layout == "business":
        elements = [
            conclusion,
            metric_div,
            {"tag": "hr"},
            markdown(f"**主要变化与下一步**\n{detail}"),
        ]
    elif layout == "trend":
        elements = [
            conclusion,
            markdown(f"**拐点与原因**\n{detail}"),
            metric_div,
        ]
    elif layout == "ranking":
        elements = [
            conclusion,
            {
                "tag": "table",
                "page_size": min(len(rows), 10),
                "columns": [
                    {"name": "item", "display_name": "项目", "data_type": "text"},
                    {"name": "value", "display_name": "结果", "data_type": "text"},
                ],
                "rows": rows,
            },
            markdown(f"**异常项**\n{detail}"),
        ]
    elif layout == "health":
        elements = [
            conclusion,
            metric_div,
            {"tag": "hr"},
            {
                "tag": "interactive_container",
                "width": "fill",
                "height": "auto",
                "corner_radius": "8px",
                "has_border": False,
                "disabled": False,
                "background_style": "orange-50",
                "padding": "10px 12px 10px 12px",
                "direction": "vertical",
                "elements": [markdown(f"**主要扣分项**\n{detail}")],
            },
        ]
    else:
        elements = [
            conclusion,
            metric_div,
            markdown(f"**风险 / 下一步**\n{detail}", text_size="notation"),
        ]

    elements.append(
        markdown(
            f"<font color='grey'>视觉预设：{style['name']} · "
            "请补充真实数据源与更新时间</font>",
            text_size="notation",
        )
    )
    return {
        "schema": "2.0",
        "config": {"update_multi": True, "summary": {"content": title}},
        "header": {
            "template": style["header_template"],
            "title": plain(title),
            "subtitle": plain(meta),
        },
        "body": {
            "direction": "vertical",
            "vertical_spacing": "medium",
            "elements": elements,
        },
    }


def semantic_starter(style: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    title = args.title or "TODO_TITLE"
    summary = args.summary or "TODO_SUMMARY"
    detail = args.detail or "TODO_DETAIL"
    template = {
        "info": "blue",
        "success": "green",
        "warning": "orange",
        "danger": "red",
        "neutral": "grey",
    }[args.header_state]
    return {
        "schema": "2.0",
        "config": {"update_multi": True, "summary": {"content": title}},
        "header": {"template": template, "title": plain(title)},
        "body": {
            "direction": "vertical",
            "vertical_spacing": "medium",
            "elements": [
                markdown(f"**{summary}**\n\n{detail}"),
                markdown(
                    "<font color='grey'>视觉预设：默认语义</font>",
                    text_size="notation",
                ),
            ],
        },
    }


def build_starter(style_id: str, args: argparse.Namespace) -> dict[str, Any]:
    style = get_style(style_id)
    if style["family"] == "custom-visual":
        return custom_starter(style, args)
    if style["style_id"] == "modern-minimal":
        return modern_starter(style, args)
    if style["family"] == "dashboard":
        return dashboard_starter(style, args)
    return semantic_starter(style, args)


def verify_assets() -> list[str]:
    errors: list[str] = []
    styles = style_catalog()
    if len({item["style_id"] for item in styles}) != len(styles):
        errors.append("style_id 存在重复")
    custom = [item for item in styles if item["family"] == "custom-visual"]
    if len(custom) != 11:
        errors.append(f"Custom Visual 主题应为 11，实际 {len(custom)}")
    for style in custom:
        palette = style["colors"]
        canvas = parse_color(palette.get("canvas", "#FFFFFF"))
        for role, background in palette.items():
            foreground = choose_foreground(background, palette)
            ratio = contrast(
                composite(parse_color(background), canvas),
                parse_color(foreground),
            )
            if ratio < 4.5:
                errors.append(
                    f"{style['style_id']} 的 on_{role} 对比度不足：{ratio:.2f}:1"
                )
        tokens = custom_theme_tokens(style)
        prefix = token_prefix(style["style_id"])
        for role in ("ink", "muted_text"):
            ratio = contrast_on(
                tokens[f"{prefix}_canvas"]["light_mode"],
                tokens[f"{prefix}_{role}"]["light_mode"],
            )
            if ratio < 4.5:
                errors.append(
                    f"{style['style_id']} starter 的 {role}/canvas 对比度不足："
                    f"{ratio:.2f}:1"
                )
    if len(DASHBOARD_PRESETS) != 6:
        errors.append(f"Dashboard 子风格应为 6，实际 {len(DASHBOARD_PRESETS)}")

    assets = asset_catalog()
    counts = {
        family: sum(item["family"] == family for item in assets)
        for family in ("header", "doodle", "phosphor")
    }
    for family, expected in {"header": 5, "doodle": 153, "phosphor": 48}.items():
        if counts[family] != expected:
            errors.append(f"{family} 素材应为 {expected}，实际 {counts[family]}")
    if len({item["asset_id"] for item in assets}) != len(assets):
        errors.append("asset_id 存在重复")
    for item in assets:
        path = Path(item["path"])
        if not path.is_file():
            errors.append(f"素材不存在：{item['asset_id']} -> {path}")
        elif path.read_bytes()[:8] != b"\x89PNG\r\n\x1a\n":
            errors.append(f"素材不是 PNG：{item['asset_id']} -> {path}")

    modern = load_json(MODERN_TOKEN_FILE)["colors"]
    for mode, base in (("light_mode", "#FFFFFF"), ("dark_mode", "#000000")):
        ratio = contrast_on(
            modern["bottom_bg"][mode],
            modern["footer_text"][mode],
            base=base,
        )
        if ratio < 4.5:
            errors.append(f"modern-minimal footer {mode} 对比度不足：{ratio:.2f}:1")
        for surface in (
            "mm_key_point_blue_surface",
            "mm_key_point_lime_surface",
            "mm_key_point_lavender_surface",
        ):
            for text_role in ("mm_key_point_ink", "mm_key_point_muted"):
                ratio = contrast_on(
                    modern[surface][mode],
                    modern[text_role][mode],
                    base=base,
                )
                if ratio < 4.5:
                    errors.append(
                        f"modern-minimal {text_role}/{surface} {mode} "
                        f"对比度不足：{ratio:.2f}:1"
                    )
    return errors


def command_styles(args: argparse.Namespace) -> None:
    styles = style_catalog()
    if args.json:
        print(json.dumps(styles, ensure_ascii=False, indent=2))
        return
    print("STYLE_ID\tFAMILY\tNAME\tVIBE")
    for item in styles:
        print(
            f"{item['style_id']}\t{item['family']}\t{item['name']}\t"
            f"{item.get('vibe', item.get('description', ''))}"
        )


def command_show(args: argparse.Namespace) -> None:
    style = get_style(args.style_id)
    if args.json:
        print(json.dumps(style, ensure_ascii=False, indent=2))
        return
    print(f"style_id: {style['style_id']}")
    print(f"family: {style['family']}")
    print(f"name: {style['name']}")
    print(f"vibe: {style.get('vibe', style.get('description', ''))}")
    if style.get("usage"):
        print("usage: " + ", ".join(style["usage"]))
    for rule in style.get("rules", []):
        print(f"- {rule}")


def command_assets(args: argparse.Namespace) -> None:
    query = args.query.lower().strip()
    items = []
    for item in asset_catalog():
        if args.family != "all" and item["family"] != args.family:
            continue
        haystack = " ".join(
            [
                item["asset_id"],
                item.get("description", ""),
                " ".join(item.get("tags", [])),
            ]
        ).lower()
        if query and not all(word in haystack for word in query.split()):
            continue
        items.append(item)
    items = items[: args.limit]
    if args.json:
        print(json.dumps(items, ensure_ascii=False, indent=2))
        return
    print("ASSET_ID\tTAGS\tPATH")
    for item in items:
        print(f"{item['asset_id']}\t{','.join(item.get('tags', []))}\t{item['path']}")


def command_copy(args: argparse.Namespace) -> None:
    asset = get_asset(args.asset_id)
    source = Path(asset["path"])
    destination = args.output
    if destination.exists() and destination.is_dir():
        destination = destination / source.name
    if (destination.exists() or destination.is_symlink()) and not args.force:
        raise ValueError(f"输出已存在：{destination}；确认覆盖时加 --force")
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.is_symlink():
        destination.unlink()
    shutil.copy2(source, destination)
    print(destination.resolve())


def command_tokens(args: argparse.Namespace) -> None:
    write_json(args.output, style_tokens(args.style_id), force=args.force)


def command_starter(args: argparse.Namespace) -> None:
    write_json(args.output, build_starter(args.style_id, args), force=args.force)


def command_verify(_: argparse.Namespace) -> None:
    errors = verify_assets()
    if errors:
        for error in errors:
            print(f"错误：{error}", file=sys.stderr)
        raise SystemExit(1)
    print("通过：19 个风格预设，5 张头图，153 个 doodle 与 48 个 phosphor 素材")


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description="飞书卡片风格与素材库工具")
    commands = root.add_subparsers(dest="command", required=True)

    styles = commands.add_parser("styles", help="列出全部风格预设")
    styles.add_argument("--json", action="store_true")
    styles.set_defaults(func=command_styles)

    show = commands.add_parser("show", help="查看一个风格预设")
    show.add_argument("style_id")
    show.add_argument("--json", action="store_true")
    show.set_defaults(func=command_show)

    assets = commands.add_parser("assets", help="检索头图和图标素材")
    assets.add_argument("--family", choices=["all", "header", "doodle", "phosphor"], default="all")
    assets.add_argument("--query", default="")
    assets.add_argument("--limit", type=int, default=20)
    assets.add_argument("--json", action="store_true")
    assets.set_defaults(func=command_assets)

    copy = commands.add_parser("copy", help="按 asset_id 原样复制素材")
    copy.add_argument("asset_id")
    copy.add_argument("--output", type=Path, required=True)
    copy.add_argument("--force", action="store_true")
    copy.set_defaults(func=command_copy)

    tokens = commands.add_parser("tokens", help="生成主题 config.style token")
    tokens.add_argument("style_id")
    tokens.add_argument("--output", type=Path)
    tokens.add_argument("--force", action="store_true")
    tokens.set_defaults(func=command_tokens)

    starter = commands.add_parser("starter", help="生成一个使用该预设的 Card JSON 2.0 起稿")
    starter.add_argument("style_id")
    starter.add_argument("--output", type=Path, required=True)
    starter.add_argument("--title")
    starter.add_argument("--summary")
    starter.add_argument("--detail")
    starter.add_argument("--meta")
    starter.add_argument("--metric", action="append", help="Dashboard 指标，格式为 标签=数值")
    starter.add_argument(
        "--header-state",
        choices=["info", "success", "warning", "danger", "neutral"],
        default="info",
    )
    starter.add_argument("--icon", help="覆盖默认素材，使用完整 asset_id")
    starter.add_argument("--force", action="store_true")
    starter.set_defaults(func=command_starter)

    verify = commands.add_parser("verify", help="校验 manifest、数量和素材文件")
    verify.set_defaults(func=command_verify)
    return root


def main(argv: list[str] | None = None) -> int:
    try:
        args = parser().parse_args(argv)
        args.func(args)
        return 0
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
