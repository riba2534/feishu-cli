#!/usr/bin/env python3
"""验证 Skill 结构与编译后二进制的命令归属。

manifest.yaml 使用 JSON 子集，避免为检查脚本引入 PyYAML 依赖。
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SKILLS = ROOT / "skills"
MANIFEST = SKILLS / "manifest.yaml"
TRIGGER_EVALS = SKILLS / "trigger-evals.json"
TRIGGER_EVAL_BUILDER = ROOT / "scripts" / "build_trigger_eval_set.py"
LINK_RE = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
RESOURCE_PATH_RE = re.compile(
    r"(?<![A-Za-z0-9_./~-])"
    r"((?:\.\./)*[A-Za-z0-9_.<>*-]+(?:/[A-Za-z0-9_.<>*-]+)+"
    r"\.(?:md|json|py|js|sh))(?![A-Za-z0-9_.-])"
)
SOURCE_SKILL_PATH_RE = re.compile(r"skills/[A-Za-z0-9_./-]+\.(?:md|json|py|js|sh)")
RESOURCE_DIRS = {"references", "scripts", "templates", "examples", "assets"}


def is_workspace_path(path: Path) -> bool:
    """评测 workspace 是运行产物，不属于发布 Skill。"""
    relative = path.relative_to(SKILLS)
    return bool(relative.parts and relative.parts[0].endswith("-workspace"))


def skill_markdown_files() -> list[Path]:
    return [path for path in SKILLS.rglob("*.md") if not is_workspace_path(path)]


def resolve_resource_path(markdown_file: Path, target: str) -> Path:
    """反引号资源路径以工作流根目录为锚点，跨 Skill 路径以仓库根为锚点。"""
    if target.startswith("skills/"):
        return ROOT / target
    if target.startswith("feishu-cli-"):
        return SKILLS / target
    if target.startswith("../"):
        return markdown_file.parent / target
    relative = markdown_file.relative_to(SKILLS)
    if "workflows" in relative.parts:
        workflow_index = relative.parts.index("workflows")
        if len(relative.parts) > workflow_index + 1:
            workflow_root = SKILLS.joinpath(*relative.parts[: workflow_index + 2])
            return workflow_root / target
    return markdown_file.parent / target


def is_resource_target(target: str) -> bool:
    normalized = target
    while normalized.startswith("../"):
        normalized = normalized[3:]
    first = normalized.split("/", 1)[0]
    return normalized.startswith("skills/") or first.startswith("feishu-cli-") or first in RESOURCE_DIRS


def fail(message: str, errors: list[str]) -> None:
    errors.append(message)


def command_info(binary: Path, path: tuple[str, ...]) -> tuple[list[str], bool]:
    proc = subprocess.run(
        [str(binary), *path, "--help"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        timeout=30,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"{' '.join(path) or '<root>'} --help 失败: {proc.stderr.strip()}")

    commands: list[str] = []
    in_section = False
    usage = ""
    lines = proc.stdout.splitlines()
    for index, line in enumerate(lines):
        if line.strip() == "Usage:":
            for candidate in lines[index + 1 :]:
                if candidate.strip():
                    usage = candidate.strip()
                    break
        if line.strip() == "Available Commands:":
            in_section = True
            continue
        if in_section and line.strip() in {"Flags:", "Global Flags:"}:
            break
        if not in_section:
            continue
        match = re.match(r"^\s{2}([a-zA-Z0-9][a-zA-Z0-9-]*)\s+", line)
        if match:
            commands.append(match.group(1))
    runnable = bool(path) and bool(usage) and "[command]" not in usage
    return commands, runnable


def collect_actionable_commands(binary: Path) -> list[tuple[str, ...]]:
    commands: list[tuple[str, ...]] = []
    stack: list[tuple[str, ...]] = [()]
    seen: set[tuple[str, ...]] = set()
    while stack:
        path = stack.pop()
        if path in seen:
            continue
        seen.add(path)
        children, runnable = command_info(binary, path)
        if path and (runnable or not children):
            commands.append(path)
        stack.extend(path + (child,) for child in reversed(children))
    return sorted(commands)


def starts_with(path: tuple[str, ...], prefix: tuple[str, ...]) -> bool:
    return path[: len(prefix)] == prefix


def check_frontmatter(skill_dir: Path, errors: list[str]) -> None:
    skill_file = skill_dir / "SKILL.md"
    if not skill_file.exists():
        fail(f"缺少 {skill_file.relative_to(ROOT)}", errors)
        return
    text = skill_file.read_text(encoding="utf-8")
    lines = text.splitlines()
    if not lines or lines[0] != "---" or "---" not in lines[1:]:
        fail(f"{skill_file.relative_to(ROOT)} frontmatter 无效", errors)
        return
    end = lines[1:].index("---") + 1
    frontmatter = "\n".join(lines[1:end])
    name_match = re.search(r"^name:\s*(\S+)\s*$", frontmatter, re.MULTILINE)
    if not name_match or name_match.group(1) != skill_dir.name:
        fail(f"{skill_file.relative_to(ROOT)} name 与目录不一致", errors)
    if not re.search(r"^description:\s*", frontmatter, re.MULTILINE):
        fail(f"{skill_file.relative_to(ROOT)} 缺少 description", errors)
    if len(lines) >= 500:
        fail(f"{skill_file.relative_to(ROOT)} 共 {len(lines)} 行，应小于 500 行", errors)


def check_links_and_toc(errors: list[str]) -> None:
    for path in skill_markdown_files():
        text = path.read_text(encoding="utf-8")
        lines = text.splitlines()
        if "references" in path.parts and len(lines) > 300:
            if not re.search(r"^##\s+(目录|Table of Contents)\s*$", text, re.MULTILINE):
                fail(f"{path.relative_to(ROOT)} 超过 300 行但没有二级目录", errors)
        for target in LINK_RE.findall(text):
            target = target.strip().split("#", 1)[0]
            if not target or target.startswith(("http://", "https://", "mailto:", "/")):
                continue
            target = target.strip("<>")
            if "$" in target or "..." in target or "://" in target:
                continue
            if Path(target).suffix.lower() not in {".md", ".json", ".py", ".js", ".sh"}:
                continue
            resolved = (path.parent / target).resolve()
            if not resolved.exists():
                fail(f"{path.relative_to(ROOT)} 存在失效链接: {target}", errors)

        for target in RESOURCE_PATH_RE.findall(text):
            if any(marker in target for marker in ("$", "...", "<", ">", "*")):
                continue
            if not is_resource_target(target):
                continue
            resolved = resolve_resource_path(path, target)
            if not resolved.is_file():
                fail(f"{path.relative_to(ROOT)} 存在失效资源路径: {target}", errors)

        for target in SOURCE_SKILL_PATH_RE.findall(text):
            if not (ROOT / target).is_file():
                fail(f"{path.relative_to(ROOT)} 引用了不存在的 Skill 文件: {target}", errors)


def check_source_skill_paths(errors: list[str]) -> None:
    for source_dir in (ROOT / "cmd", ROOT / "internal"):
        for path in source_dir.rglob("*.go"):
            for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                for target in SOURCE_SKILL_PATH_RE.findall(line):
                    if not (ROOT / target).is_file():
                        fail(
                            f"{path.relative_to(ROOT)}:{line_number} 引用了不存在的 Skill 文件: {target}",
                            errors,
                        )


def check_eval_coverage(
    expected_skills: set[str], declared_workflows: set[tuple[str, str]], errors: list[str]
) -> None:
    covered_workflows: set[tuple[str, str]] = set()
    for skill in sorted(expected_skills):
        eval_file = SKILLS / skill / "evals" / "evals.json"
        if not eval_file.is_file():
            fail(f"缺少评测文件: {eval_file.relative_to(ROOT)}", errors)
            continue
        eval_data = json.loads(eval_file.read_text(encoding="utf-8"))
        if eval_data.get("skill_name") != skill:
            fail(f"{eval_file.relative_to(ROOT)} skill_name 与目录不一致", errors)
        evals = eval_data.get("evals")
        if not isinstance(evals, list) or not evals:
            fail(f"{eval_file.relative_to(ROOT)} 没有评测用例", errors)
            continue
        for case in evals:
            workflows = case.get("workflows")
            if not isinstance(workflows, list) or not workflows:
                fail(f"{eval_file.relative_to(ROOT)} 的 eval {case.get('id')} 未声明 workflows", errors)
                continue
            covered_workflows.update((skill, workflow) for workflow in workflows)

    if covered_workflows != declared_workflows:
        fail(
            "工作流评测覆盖不完整: "
            f"未覆盖={sorted('/'.join(item) for item in declared_workflows - covered_workflows)}, "
            f"无效={sorted('/'.join(item) for item in covered_workflows - declared_workflows)}",
            errors,
        )


def check_trigger_evals(
    manifest: dict[str, object], declared_workflows: set[tuple[str, str]], errors: list[str]
) -> None:
    expected_items = manifest.get("legacy_skill_mappings", [])
    if not isinstance(expected_items, list):
        fail("manifest legacy_skill_mappings 必须是数组", errors)
        return
    expected_mappings = {
        (item["legacy_skill"], item["skill"], item["workflow"])
        for item in expected_items
    }
    expected_legacy_skills = {mapping[0] for mapping in expected_mappings}
    if len(expected_legacy_skills) != 29:
        fail("manifest 必须覆盖全部 29 个 legacy Skill", errors)
    if len(expected_items) != len(expected_mappings):
        fail("manifest legacy Skill 映射存在重复项", errors)
    for _, skill, workflow in expected_mappings:
        if (skill, workflow) not in declared_workflows:
            fail(f"manifest legacy 映射指向未声明工作流: {skill}/{workflow}", errors)

    trigger_items = json.loads(TRIGGER_EVALS.read_text(encoding="utf-8"))
    if not isinstance(trigger_items, list):
        fail(f"{TRIGGER_EVALS.relative_to(ROOT)} 必须是数组", errors)
        return
    actual_mappings: list[tuple[str, str, str]] = []
    queries: set[str] = set()
    positive_counts = {skill: 0 for skill in manifest["expected_top_level_skills"]}
    for index, item in enumerate(trigger_items, 1):
        query = item.get("query")
        if not isinstance(query, str) or not query.strip():
            fail(f"trigger eval #{index} 缺少 query", errors)
        elif query in queries:
            fail(f"trigger eval #{index} query 重复", errors)
        else:
            queries.add(query)
        if "should_trigger" in item:
            fail(f"trigger eval #{index} 不应固定 should_trigger；由目标 Skill 动态派生", errors)

        target = (item.get("expected_skill"), item.get("expected_workflow"))
        if not all(isinstance(value, str) and value for value in target):
            fail(f"trigger eval #{index} 的 skill/workflow 映射无效", errors)
            continue
        if target not in declared_workflows:
            fail(f"trigger eval #{index} 指向未声明工作流: {'/'.join(target)}", errors)
            continue
        positive_counts[target[0]] += 1

        legacy_skill = item.get("legacy_skill")
        if legacy_skill is not None:
            if not isinstance(legacy_skill, str) or not legacy_skill:
                fail(f"trigger eval #{index} 的 legacy_skill 无效", errors)
                continue
            actual_mappings.append((legacy_skill, target[0], target[1]))

    if len(trigger_items) != 72 or any(count != 8 for count in positive_counts.values()):
        fail(f"触发路由评测必须为每个领域提供 8 个正例: {positive_counts}", errors)

    actual_mapping_set = set(actual_mappings)
    actual_legacy_skills = {mapping[0] for mapping in actual_mapping_set}
    if len(actual_mappings) != len(actual_mapping_set):
        fail(f"{TRIGGER_EVALS.relative_to(ROOT)} 存在重复 legacy 映射", errors)
    if len(actual_legacy_skills) != 29:
        fail(f"{TRIGGER_EVALS.relative_to(ROOT)} 必须覆盖全部 29 个 legacy Skill", errors)
    if actual_mapping_set != expected_mappings:
        fail(
            "trigger eval legacy 映射与 manifest 不一致: "
            f"缺少={sorted(expected_mappings - actual_mapping_set)}, "
            f"多出={sorted(actual_mapping_set - expected_mappings)}",
            errors,
        )

    for skill in manifest["expected_top_level_skills"]:
        proc = subprocess.run(
            [sys.executable, str(TRIGGER_EVAL_BUILDER), skill],
            cwd=ROOT,
            text=True,
            capture_output=True,
            timeout=30,
            check=False,
        )
        if proc.returncode != 0:
            fail(f"{skill} 触发评测集生成失败: {proc.stderr.strip()}", errors)
            continue
        generated = json.loads(proc.stdout)
        positive = sum(item.get("should_trigger") is True for item in generated)
        negative = sum(item.get("should_trigger") is False for item in generated)
        if len(generated) != 16 or positive != 8 or negative != 8:
            fail(f"{skill} 触发评测集不是 8 正例 + 8 近邻负例", errors)


def main() -> int:
    binary = Path(sys.argv[1] if len(sys.argv) > 1 else ROOT / "feishu-cli").resolve()
    if not binary.exists():
        print(f"错误: 找不到编译产物 {binary}", file=sys.stderr)
        return 2

    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    errors: list[str] = []
    expected = set(manifest["expected_top_level_skills"])
    actual = {
        p.name
        for p in SKILLS.iterdir()
        if p.is_dir() and not p.name.endswith("-workspace") and (p / "SKILL.md").exists()
    }
    if actual != expected:
        fail(f"顶层 Skill 不一致: 缺少={sorted(expected - actual)}, 多出={sorted(actual - expected)}", errors)

    nested = [
        p
        for p in SKILLS.rglob("SKILL.md")
        if not is_workspace_path(p) and p.parent.parent != SKILLS
    ]
    if nested:
        fail("发现嵌套 SKILL.md: " + ", ".join(str(p.relative_to(ROOT)) for p in nested), errors)

    for name in sorted(expected):
        check_frontmatter(SKILLS / name, errors)

    owners: list[tuple[str, str, tuple[str, ...]]] = []
    owned_workflows: set[tuple[str, str]] = set()
    for owner in manifest["owners"]:
        workflow_key = (owner["skill"], owner["workflow"])
        if workflow_key in owned_workflows:
            fail(f"manifest 重复声明命令工作流: {'/'.join(workflow_key)}", errors)
        owned_workflows.add(workflow_key)
        workflow = SKILLS / owner["skill"] / "references" / "workflows" / owner["workflow"] / "workflow.md"
        if not workflow.exists():
            fail(f"manifest 指向不存在的工作流: {workflow.relative_to(ROOT)}", errors)
        for prefix in owner["prefixes"]:
            owners.append((owner["skill"], owner["workflow"], tuple(prefix)))

    non_command_workflows: set[tuple[str, str]] = set()
    for item in manifest.get("non_command_workflows", []):
        workflow_key = (item["skill"], item["workflow"])
        if workflow_key in non_command_workflows:
            fail(f"manifest 重复声明非命令工作流: {'/'.join(workflow_key)}", errors)
        non_command_workflows.add(workflow_key)
        workflow = SKILLS / item["skill"] / "references" / "workflows" / item["workflow"] / "workflow.md"
        if not workflow.exists():
            fail(f"manifest 指向不存在的非命令工作流: {workflow.relative_to(ROOT)}", errors)

    overlap = owned_workflows & non_command_workflows
    if overlap:
        fail(f"工作流同时声明为命令与非命令: {sorted('/'.join(item) for item in overlap)}", errors)

    actual_workflows = {
        (path.parents[3].name, path.parent.name)
        for path in SKILLS.glob("*/references/workflows/*/workflow.md")
        if not is_workspace_path(path)
    }
    declared_workflows = owned_workflows | non_command_workflows
    if actual_workflows != declared_workflows:
        fail(
            "工作流 manifest 覆盖不完整: "
            f"未声明={sorted('/'.join(item) for item in actual_workflows - declared_workflows)}, "
            f"不存在={sorted('/'.join(item) for item in declared_workflows - actual_workflows)}",
            errors,
        )
    check_eval_coverage(expected, declared_workflows, errors)
    check_trigger_evals(manifest, declared_workflows, errors)

    excluded = [tuple(prefix) for prefix in manifest["excluded_command_prefixes"]]
    hidden_commands = [tuple(path) for path in manifest.get("hidden_commands", [])]
    for hidden in hidden_commands:
        children, runnable = command_info(binary, hidden)
        if not runnable and children:
            fail(f"manifest hidden command 不是可执行命令: {' '.join(hidden)}", errors)
    leaves = sorted(set(collect_actionable_commands(binary)) | set(hidden_commands))
    covered = 0
    for leaf in leaves:
        if any(starts_with(leaf, prefix) for prefix in excluded):
            continue
        matches = [owner for owner in owners if starts_with(leaf, owner[2])]
        if not matches:
            fail(f"命令没有归属: {' '.join(leaf)}", errors)
            continue
        longest = max(len(match[2]) for match in matches)
        winners = {(skill, workflow) for skill, workflow, prefix in matches if len(prefix) == longest}
        if len(winners) != 1:
            fail(f"命令存在多个同优先级归属: {' '.join(leaf)} -> {sorted(winners)}", errors)
            continue
        covered += 1

    check_links_and_toc(errors)
    check_source_skill_paths(errors)

    if errors:
        print(f"Skill 检查失败，共 {len(errors)} 项:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print(
        f"Skill 检查通过: {len(expected)} 个顶层 Skill，{len(declared_workflows)} 个工作流均有评测，"
        f"29 个 legacy Skill 迁移映射完整，9 组触发评测各含 8 正例/8 近邻负例，"
        f"{covered} 个可执行业务命令全部唯一归属。"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
