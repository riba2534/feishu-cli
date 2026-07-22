#!/usr/bin/env bash
set -euo pipefail

REPO="riba2534/feishu-cli"
BINARY_NAME="feishu-cli"
DEFAULT_INSTALL_DIR="/usr/local/bin"
tmpdir=""

# 颜色输出（全部走 stderr：stdout 仅保留给函数通过 $(...) 返回数据，
# 避免日志被 version=$(get_latest_version) 之类的命令替换捕获，污染下载 URL）
info()  { printf "\033[34m[INFO]\033[0m  %s\n" "$*" >&2; }
ok()    { printf "\033[32m[OK]\033[0m    %s\n" "$*" >&2; }
warn()  { printf "\033[33m[WARN]\033[0m  %s\n" "$*" >&2; }
err()   { printf "\033[31m[ERROR]\033[0m %s\n" "$*" >&2; }

# 检测操作系统
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       err "不支持的操作系统: $(uname -s)"; exit 1 ;;
    esac
}

# 检测架构
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             err "不支持的架构: $(uname -m)"; exit 1 ;;
    esac
}

# 获取最新版本号
#
# 优先使用 302 redirect 方式（https://github.com/OWNER/REPO/releases/latest
# 会 302 跳转到 /releases/tag/vX.Y.Z，从 Location header 提取 tag 即可），
# 这个路径走的是 github.com 网页路由而不是 api.github.com，**不消耗 API 配额**。
#
# Fallback 到 GitHub API（未认证 60 次/小时配额；设置 GITHUB_TOKEN 可提升到 5000 次/小时）。
# 修复 #90: GitHub API 未认证配额共享同一出口 IP，多人共享网络/CI/容器场景下极易 403。
get_latest_version() {
    local version=""
    local redirect_url="https://github.com/${REPO}/releases/latest"

    # 方式 1：通过 302 redirect 提取 tag（零配额消耗）
    if command -v curl &>/dev/null; then
        version=$(curl -sI "$redirect_url" 2>/dev/null \
            | grep -i '^location:' \
            | head -1 \
            | sed 's|.*/tag/\([^[:space:]]*\).*|\1|' \
            | tr -d '\r\n')
    elif command -v wget &>/dev/null; then
        version=$(wget --spider -S "$redirect_url" 2>&1 \
            | grep -i '^[[:space:]]*location:' \
            | head -1 \
            | sed 's|.*/tag/\([^[:space:]]*\).*|\1|' \
            | tr -d '\r\n')
    fi

    # 方式 2：fallback 到 GitHub API（60 次/小时；GITHUB_TOKEN 可提到 5000 次/小时）
    if [ -z "$version" ]; then
        info "302 redirect 未获取到版本号，回退到 GitHub API..."
        local api_url="https://api.github.com/repos/${REPO}/releases/latest"
        local token="${GITHUB_TOKEN:-}"
        if command -v curl &>/dev/null; then
            if [ -n "$token" ]; then
                version=$(curl -fsSL -H "Authorization: Bearer $token" "$api_url" 2>/dev/null \
                    | grep '"tag_name"' | head -1 \
                    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
            else
                version=$(curl -fsSL "$api_url" 2>/dev/null \
                    | grep '"tag_name"' | head -1 \
                    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
            fi
        elif command -v wget &>/dev/null; then
            if [ -n "$token" ]; then
                version=$(wget -qO- --header="Authorization: Bearer $token" "$api_url" \
                    | grep '"tag_name"' | head -1 \
                    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
            else
                version=$(wget -qO- "$api_url" \
                    | grep '"tag_name"' | head -1 \
                    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
            fi
        else
            err "需要 curl 或 wget"
            exit 1
        fi
    fi

    if [ -z "$version" ]; then
        err "无法获取最新版本号（GitHub API 可能已达速率限制，可设置 GITHUB_TOKEN 环境变量提升配额到 5000/小时）"
        exit 1
    fi

    # 去除可能混入的空白/控制字符，并校验版本号格式（防御性兜底）：
    # 一旦版本号被异常内容污染（如日志、HTML），此处提前报错，
    # 而不是带着脏字符去拼接 download_url 让 curl 报 "bad range in URL"。
    version=$(printf '%s' "$version" | tr -d '[:space:][:cntrl:]')
    if ! printf '%s' "$version" | grep -qE '^v?[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'; then
        err "获取到的版本号格式异常: '${version}'（期望形如 v1.2.3）"
        exit 1
    fi

    echo "$version"
}

# 检测安装目录
# 优先级：已有安装位置 > GOPATH/bin > GOBIN > /usr/local/bin
detect_install_dir() {
    # 1. 如果已安装，更新到同一位置
    local existing
    existing=$(command -v "$BINARY_NAME" 2>/dev/null || true)
    if [ -n "$existing" ]; then
        # 解析符号链接，获取真实路径的目录
        local real_path
        real_path=$(readlink -f "$existing" 2>/dev/null || echo "$existing")
        echo "$(dirname "$real_path")"
        return
    fi

    # 2. 检查 GOBIN
    if [ -n "${GOBIN:-}" ] && [ -d "$GOBIN" ]; then
        echo "$GOBIN"
        return
    fi

    # 3. 检查 GOPATH/bin
    local gopath_bin
    if [ -n "${GOPATH:-}" ]; then
        gopath_bin="${GOPATH}/bin"
    elif command -v go &>/dev/null; then
        gopath_bin="$(go env GOPATH 2>/dev/null)/bin"
    fi
    if [ -n "${gopath_bin:-}" ] && [ -d "$gopath_bin" ]; then
        echo "$gopath_bin"
        return
    fi

    # 4. 默认
    echo "$DEFAULT_INSTALL_DIR"
}

# 计算文件的 sha256（优先 sha256sum，回退 shasum）；无可用工具时返回空
compute_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        echo ""
    fi
}

# 校验下载的安装包 sha256。
# release 若提供 checksums.txt 则强制校验，不匹配立即终止安装；
# 旧 release 无 checksums.txt 时告警但继续（向后兼容）。
verify_checksum() {
    local dir="$1" asset="$2" version="$3"
    local checksums_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"
    local checksums_file="${dir}/checksums.txt"

    info "校验安装包完整性..."

    # 下载 checksums.txt（404/网络失败视为该 release 未提供校验文件）
    local downloaded=0
    if command -v curl &>/dev/null; then
        curl -fsSL -o "$checksums_file" "$checksums_url" 2>/dev/null && downloaded=1
    elif command -v wget &>/dev/null; then
        wget -qO "$checksums_file" "$checksums_url" 2>/dev/null && downloaded=1
    fi
    if [ "$downloaded" -ne 1 ] || [ ! -s "$checksums_file" ]; then
        warn "该版本未提供 checksums.txt，跳过完整性校验（旧版本兼容）"
        return 0
    fi

    # 提取该资产对应的期望值（sha256sum 文本模式输出：<hash>  <filename>）
    local expected
    expected=$(awk -v f="$asset" '$NF==f {print $1; exit}' "$checksums_file")
    if [ -z "$expected" ]; then
        warn "checksums.txt 中未找到 ${asset} 的校验值，跳过校验"
        return 0
    fi

    local actual
    actual=$(compute_sha256 "${dir}/${asset}")
    if [ -z "$actual" ]; then
        warn "系统缺少 sha256sum/shasum，无法校验完整性，跳过"
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        err "安装包完整性校验失败！sha256 不匹配"
        err "  期望: ${expected}"
        err "  实际: ${actual}"
        err "已终止安装，请重试或从官方 release 页面手动下载"
        exit 1
    fi
    ok "完整性校验通过（sha256 匹配）"
}

# 下载并安装
install() {
    local os arch version install_dir asset_name download_url

    os=$(detect_os)
    arch=$(detect_arch)
    version=$(get_latest_version)
    install_dir=$(detect_install_dir)

    info "检测到平台: ${os}/${arch}"
    info "最新版本: ${version}"
    info "安装目录: ${install_dir}"

    # 检查是否已安装相同版本
    if command -v "$BINARY_NAME" &>/dev/null; then
        local current
        current=$("$BINARY_NAME" --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
        if [ "$current" = "$version" ]; then
            ok "已是最新版本 ${version}，无需更新"
            exit 0
        fi
        info "当前版本: ${current}，将更新到 ${version}"
    fi

    # 构造资产文件名
    asset_name="${BINARY_NAME}_${version}_${os}-${arch}.tar.gz"
    download_url="https://github.com/${REPO}/releases/download/${version}/${asset_name}"

    # 创建临时目录
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    info "下载 ${download_url}"
    if command -v curl &>/dev/null; then
        curl -fSL --progress-bar -o "${tmpdir}/${asset_name}" "$download_url"
    else
        wget -q --show-progress -O "${tmpdir}/${asset_name}" "$download_url"
    fi

    # 校验完整性（release 提供 checksums.txt 时强制，不匹配则终止）
    verify_checksum "$tmpdir" "$asset_name" "$version"

    info "解压安装包..."
    tar -xzf "${tmpdir}/${asset_name}" -C "$tmpdir"

    # 查找二进制文件（可能在子目录中）
    local binary_path
    binary_path=$(find "$tmpdir" -name "$BINARY_NAME" -type f | head -1)
    if [ -z "$binary_path" ]; then
        err "解压后未找到 ${BINARY_NAME} 二进制文件"; exit 1
    fi
    chmod +x "$binary_path"

    # 安装到目标目录
    info "安装到 ${install_dir}/${BINARY_NAME}"
    if [ -w "$install_dir" ]; then
        mv "$binary_path" "${install_dir}/${BINARY_NAME}"
    else
        sudo mv "$binary_path" "${install_dir}/${BINARY_NAME}"
    fi

    # 验证安装
    if command -v "$BINARY_NAME" &>/dev/null; then
        ok "安装成功: $("$BINARY_NAME" --version 2>/dev/null)"
    else
        ok "已安装到 ${install_dir}/${BINARY_NAME}"
        echo "  如果命令未找到，请确认 ${install_dir} 在 PATH 中"
    fi
}

install
