#!/usr/bin/env bash
# 按 CLAUDE.md「发布 Release 规范」一步产出 5 个平台安装包与 checksums.txt，并做结构自检。
#
# 用法: scripts/release_package.sh <vX.Y.Z> [输出目录，默认 dist]
#   make release-package VERSION=vX.Y.Z
#
# 产物（与 install.sh 的资产命名一致）:
#   <out>/feishu-cli_<版本>_<os>-<arch>.tar.gz   内含同名目录/feishu-cli（Windows 为 feishu-cli.exe）
#   <out>/checksums.txt                          sha256sum 文本格式
set -euo pipefail

VERSION="${1:-}"
OUT="${2:-dist}"
PLATFORMS=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64)

die() { printf '错误: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*" >&2; }

[ -n "$VERSION" ] || die "缺少版本号。用法: $0 vX.Y.Z [输出目录]"
if ! printf '%s' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    die "版本号格式不对: '$VERSION'（期望 vX.Y.Z 或 vX.Y.Z-rc1）"
fi
case "$VERSION" in
    *dirty*|*-[0-9]*-g[0-9a-f]*) die "版本号 '$VERSION' 像 git describe 的开发版本；发版请显式传 tag，如 VERSION=v1.42.0" ;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

sha256_cmd() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$@"
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$@"
    else
        die "需要 sha256sum 或 shasum"
    fi
}

TAR_FLAGS=()
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    TAR_FLAGS=(--owner=0 --group=0 --numeric-owner --sort=name)
fi

# 只清理本版本的旧产物，不动输出目录里的其他文件
rm -rf "$OUT"/feishu-cli_"$VERSION"_* "$OUT/checksums.txt"

BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S')"
LDFLAGS="-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME"
ASSETS=()

for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    name="feishu-cli_${VERSION}_${os}-${arch}"
    bin="feishu-cli"
    [ "$os" = "windows" ] && bin="feishu-cli.exe"
    stage="$OUT/$name"
    mkdir -p "$stage"
    info "构建 $platform → $name/$bin"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "$stage/$bin" .
    (cd "$OUT" && tar "${TAR_FLAGS[@]}" -czf "$name.tar.gz" "$name")
    rm -rf "$stage"
    ASSETS+=("$name.tar.gz")
done

(cd "$OUT" && sha256_cmd "${ASSETS[@]}" > checksums.txt)

# ---------- 结构自检 ----------
info "自检产物结构"
[ "$(wc -l < "$OUT/checksums.txt" | tr -d ' ')" = "${#PLATFORMS[@]}" ] || die "checksums.txt 行数不等于平台数"
(cd "$OUT" && sha256_cmd -c checksums.txt >/dev/null) || die "checksums.txt 校验失败"

host_os="$(go env GOOS)"
host_arch="$(go env GOARCH)"
for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    name="feishu-cli_${VERSION}_${os}-${arch}"
    bin="feishu-cli"
    [ "$os" = "windows" ] && bin="feishu-cli.exe"
    archive="$OUT/$name.tar.gz"
    [ -f "$archive" ] || die "缺少 $archive"
    entries="$(tar -tzf "$archive" | sed 's|/$||' | sort | tr '\n' ' ')"
    expected="$(printf '%s\n%s\n' "$name" "$name/$bin" | sort | tr '\n' ' ')"
    [ "$entries" = "$expected" ] || die "$name.tar.gz 结构不符合规范: [$entries]，期望 [$expected]"
    grep -q "  $name.tar.gz\$" "$OUT/checksums.txt" || die "checksums.txt 缺少 $name.tar.gz"

    if [ "$os" = "$host_os" ] && [ "$arch" = "$host_arch" ]; then
        tmp="$(mktemp -d)"
        tar -xzf "$archive" -C "$tmp"
        exe="$tmp/$name/$bin"
        [ -x "$exe" ] || die "$name/$bin 没有可执行权限"
        got="$("$exe" --version)"
        case "$got" in
            *"$VERSION"*) ;;
            *) die "$name/$bin --version 输出 '$got' 不含 $VERSION" ;;
        esac
        "$exe" skills list --json | grep -q '"count": 9' || die "$name/$bin 未内嵌 9 个技能"
        rm -rf "$tmp"
        info "宿主平台 $platform 实跑通过: $got"
    fi
done

info "完成: $OUT"
(cd "$OUT" && ls -l "${ASSETS[@]}" checksums.txt) >&2
