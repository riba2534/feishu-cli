#!/usr/bin/env python3
"""为单个领域 Skill 生成 skill-creator 可直接执行的触发评测集。"""

from __future__ import annotations

import argparse
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DATA = ROOT / "skills" / "trigger-evals.json"
MANIFEST = ROOT / "skills" / "manifest.yaml"


def build_eval_set(skill: str, rows: list[dict[str, object]], skills: list[str]) -> list[dict[str, object]]:
    """返回 8 个正例和 8 个同产品近邻负例。

    每个其他飞书领域贡献一个负例。它们都属于 feishu-cli，但不属于目标领域，能验证
    领域整合后的边界，而不是用天气等无关请求制造过于容易的负例。
    """
    positives = [row for row in rows if row.get("expected_skill") == skill]
    if len(positives) != 8:
        raise ValueError(f"{skill} 必须恰好有 8 个正例，当前 {len(positives)} 个")

    negatives: list[dict[str, object]] = []
    target_index = skills.index(skill)
    for source_index, source_skill in enumerate(skills):
        if source_skill == skill:
            continue
        candidates = [row for row in rows if row.get("expected_skill") == source_skill]
        if len(candidates) != 8:
            raise ValueError(f"{source_skill} 必须恰好有 8 个候选负例")
        negatives.append(candidates[(target_index + source_index) % len(candidates)])

    return [
        *({"query": row["query"], "should_trigger": True} for row in positives),
        *({"query": row["query"], "should_trigger": False} for row in negatives),
    ]


def main() -> int:
    parser = argparse.ArgumentParser(description="生成单个领域 Skill 的触发评测 JSON")
    parser.add_argument("skill", help="manifest 中的顶层 Skill 名称")
    parser.add_argument("--data", type=Path, default=DEFAULT_DATA, help="路由评测源数据")
    args = parser.parse_args()

    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    skills = manifest["expected_top_level_skills"]
    if args.skill not in skills:
        parser.error(f"未知 Skill: {args.skill}")

    rows = json.loads(args.data.read_text(encoding="utf-8"))
    result = build_eval_set(args.skill, rows, skills)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
