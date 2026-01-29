#!/usr/bin/env bash
set -euo pipefail

REPO="riba2534/feishu-cli"
BINARY_NAME="feishu-cli"
DEFAULT_INSTALL_DIR="/usr/local/bin"

# 颜色输出
info()  { printf "\033[34m[INFO]\033[0m  %s\n" "$*"; }
ok()    { printf "\033[32m[OK]\033[0m    %s\n" "$*"; }
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
get_latest_version() {
    local url="https://api.github.com/repos/${REPO}/releases/latest"
    local version

    if command -v curl &>/dev/null; then
        version=$(curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    elif command -v wget &>/dev/null; then
        version=$(wget -qO- "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    else
        err "需要 curl 或 wget"; exit 1
    fi

    if [ -z "$version" ]; then
        err "无法获取最新版本号"; exit 1
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

# 下载并安装
install() {
    local os arch version install_dir asset_name download_url tmpdir

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
