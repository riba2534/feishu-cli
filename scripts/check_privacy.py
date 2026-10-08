#!/usr/bin/env python3
"""扫描仓库文件中的隐私泄漏（开源项目提交前 / CI 必跑）。

规则对应 CLAUDE.md「隐私安全」条款：
  internal-email   内部邮箱（字节 / 飞书 / Lark 域名下的真实邮箱地址）
  tenant-domain    带企业前缀的飞书域名（<企业>.feishu.cn 等）；官方子域与占位前缀除外
  app-id           真实形态的 App ID（cli_ + 16 位十六进制）
  user-token       User / Tenant / Refresh Token 形态（u- / t- / ur- 前缀的高熵串）
  app-secret       App Secret 赋值（32 位字母数字）
  jwt              JWT 形态的 access token
  github-token     GitHub 个人 token

默认扫描 git 已跟踪 + 未忽略的新文件（git ls-files --cached --others --exclude-standard），
跳过二进制、符号链接与上游生成数据。单行误报可在该行加 `privacy-scan: allow` 注释豁免。
只用标准库，CI 无需额外依赖。
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

# 上游公开数据：make update-meta 从 open.feishu.cn 公开接口生成，内含官方文档示例值
EXCLUDED_PATHS = {
    "internal/registry/meta_data.json",
}

ALLOW_MARKER = "privacy-scan: allow"

# 官方子域与文档占位前缀（新增占位前缀请保持“一眼可知是假的”）
OFFICIAL_SUBDOMAINS = {
    "open", "www", "accounts", "passport", "applink", "api-drive-stream", "base-api",
    "miaoda", "vc", "meetings", "calendar", "internal-api-lark-api", "docs", "f", "s",
}
PLACEHOLDER_SUBDOMAINS = {
    "xxx", "xxxx", "sample", "example", "examples", "custom", "tenant", "your-tenant",
    "yourtenant", "your-domain", "yourdomain", "your-company", "yourcompany", "company",
    "demo", "test", "foo", "bar", "abc", "my", "a", "b", "domain", "tenant-name", "mycompany",
}

# 文档里用作“禁止写法”反例的固定文本：(相对路径, 命中文本)
# 用拼接避免本脚本自身被扫描命中
KNOWN_EXAMPLES = {
    ("CLAUDE.md", "bytedance" + ".feishu.cn"),
}

FEISHU_DOMAINS = r"(?:feishu\.cn|larkoffice\.com|larksuite\.com|feishu-boe\.cn|larkenterprise\.com)"
INTERNAL_EMAIL_DOMAINS = (
    r"(?:bytedance\.(?:com|net|org)|byted\.org|bytedance-inc\.com|lark\.com|larksuite\.com"
    r"|feishu\.cn|larkoffice\.com)"
)

PLACEHOLDER_WORDS = re.compile(
    r"(?i)(x{4,}|example|sample|placeholder|dummy|fake|mock|test|secret|token|redacted|your|demo|abcdef|0123456789)"
)


@dataclass
class Finding:
    path: str
    line: int
    rule: str
    text: str

    def render(self) -> str:
        return f"{self.path}:{self.line}: [{self.rule}] {redact(self.text)}"


def redact(text: str) -> str:
    if len(text) <= 12:
        return text
    return text[:8] + "…" + text[-4:]


def high_entropy(value: str) -> bool:
    return bool(re.search(r"[A-Z]", value) and re.search(r"[a-z]", value) and re.search(r"[0-9]", value))


def looks_placeholder(value: str) -> bool:
    if PLACEHOLDER_WORDS.search(value):
        return True
    core = re.sub(r"[^A-Za-z0-9]", "", value)
    return len(set(core.lower())) <= 4


EMAIL_RE = re.compile(r"(?<![A-Za-z0-9._%+-])([A-Za-z0-9._%+-]+@(?:[A-Za-z0-9-]+\.)*" + INTERNAL_EMAIL_DOMAINS + r")(?![A-Za-z0-9-])", re.I)
DOMAIN_RE = re.compile(r"(?<![A-Za-z0-9-])((?:[A-Za-z0-9-]+\.)*?([A-Za-z0-9][A-Za-z0-9-]*)\." + FEISHU_DOMAINS + r")(?![A-Za-z0-9-])", re.I)
APP_ID_RE = re.compile(r"(?<![A-Za-z0-9_])(cli_[0-9a-f]{16})(?![A-Za-z0-9_])")
TOKEN_RE = re.compile(r"(?<![A-Za-z0-9_-])((?:u|t|ur)-[A-Za-z0-9._-]{20,})")
SECRET_RE = re.compile(r"(?i)app[_-]?secret[\"']?\s*(?:[:=]|\s)\s*[\"']?([A-Za-z0-9]{32})(?![A-Za-z0-9])")
URL_AUTHORITY_PREFIX = re.compile(r"//[^\s/@]*$")
JWT_RE = re.compile(r"(eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})")
GITHUB_RE = re.compile(r"(?<![A-Za-z0-9_])((?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{60,})")


def scan_line(path: str, number: int, line: str) -> list[Finding]:
    if ALLOW_MARKER in line:
        return []
    found: list[Finding] = []

    def add(rule: str, text: str) -> None:
        if (path, text) in KNOWN_EXAMPLES:
            return
        found.append(Finding(path, number, rule, text))

    for match in EMAIL_RE.finditer(line):
        # URL userinfo（https://user:pass@host）不是邮箱；其 host 由 tenant-domain 规则检查
        if URL_AUTHORITY_PREFIX.search(line[: match.start()]):
            continue
        add("internal-email", match.group(1))
    for match in DOMAIN_RE.finditer(line):
        host, label = match.group(1), match.group(2).lower()
        # 邮箱的域名部分已由 internal-email 规则负责
        if match.start() > 0 and line[match.start() - 1] == "@":
            continue
        if label in OFFICIAL_SUBDOMAINS or label in PLACEHOLDER_SUBDOMAINS:
            continue
        add("tenant-domain", host)
    for match in APP_ID_RE.finditer(line):
        if not looks_placeholder(match.group(1)[4:]):
            add("app-id", match.group(1))
    for match in TOKEN_RE.finditer(line):
        value = match.group(1)
        if high_entropy(value) and not looks_placeholder(value):
            add("user-token", value)
    for match in SECRET_RE.finditer(line):
        value = match.group(1)
        if high_entropy(value) and not looks_placeholder(value):
            add("app-secret", value)
    for match in JWT_RE.finditer(line):
        add("jwt", match.group(1))
    for match in GITHUB_RE.finditer(line):
        add("github-token", match.group(1))
    return found


def scan_text(path: str, text: str) -> list[Finding]:
    findings: list[Finding] = []
    for number, line in enumerate(text.splitlines(), 1):
        findings.extend(scan_line(path, number, line))
    return findings


def candidate_files(paths: list[str]) -> list[str]:
    if paths:
        return paths
    proc = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT, capture_output=True, check=True,
    )
    return sorted({p for p in proc.stdout.decode("utf-8", "replace").split("\0") if p})


def scan_file(rel: str) -> list[Finding]:
    if rel in EXCLUDED_PATHS:
        return []
    path = ROOT / rel
    if path.is_symlink() or not path.is_file():
        return []
    if path.stat().st_size > 5 * 1024 * 1024:
        return []
    data = path.read_bytes()
    if b"\0" in data[:8192]:
        return []
    return scan_text(rel, data.decode("utf-8", "replace"))


def main() -> int:
    parser = argparse.ArgumentParser(description="扫描隐私泄漏（内部邮箱、企业前缀域名、真实 token）")
    parser.add_argument("paths", nargs="*", help="只扫描指定的仓库相对路径（默认全部已跟踪与未忽略文件）")
    args = parser.parse_args()
    files = candidate_files(args.paths)
    findings: list[Finding] = []
    for rel in files:
        findings.extend(scan_file(rel))
    if findings:
        print(f"隐私扫描失败：发现 {len(findings)} 处疑似敏感信息（已脱敏显示）:", file=sys.stderr)
        for finding in findings:
            print("  " + finding.render(), file=sys.stderr)
        print(
            "\n处理建议：邮箱改为 user@example.com，域名改为 xxx.feishu.cn / open.feishu.cn，"
            "Token 改为 cli_xxx / u-xxx 等占位符；确属误报时在该行加 `privacy-scan: allow` 注释。",
            file=sys.stderr,
        )
        return 1
    print(f"隐私扫描通过：{len(files)} 个文件，未发现内部邮箱、企业前缀域名或疑似真实 token。")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
