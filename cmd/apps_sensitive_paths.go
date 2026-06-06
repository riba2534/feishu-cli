package cmd

import (
	"path/filepath"
	"strings"
)

// 凭证文件扫描 —— 移植自官方 lark-cli shortcuts/apps/sensitive_paths.go。
//
// 范围刻意收窄：只拦截「约定俗成持有 API token / 服务凭证」的文件，不覆盖
// 「任何加密物」。SSH 私钥、通用 *.pem / *.key、SCM 内部文件不在此列。

// appsIsSensitiveRelPath 判断相对路径是否是知名 env / 凭证文件。
// 按 "/" 分段逐段检查，嵌套在任意子目录下的凭证文件也能命中。
func appsIsSensitiveRelPath(rel string) bool {
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		switch {
		case p == ".env" || strings.HasPrefix(p, ".env."):
			return true
		case p == ".npmrc":
			return true
		case p == ".netrc":
			return true
		case p == ".git-credentials":
			return true
		}
		if i == 0 {
			continue
		}
		switch parts[i-1] {
		case ".aws":
			if p == "credentials" {
				return true
			}
		case ".docker":
			if p == "config.json" {
				return true
			}
		case ".kube":
			if p == "config" {
				return true
			}
		}
	}
	return false
}

// appsHasParentAnchoredCredentialPair 只扫依赖父目录约定的云 SDK 凭证对
// （.aws/credentials、.docker/config.json、.kube/config）。叶子名匹配器
// （.env / .npmrc / …）刻意不在这里跑，以便调用方探测带根上下文的路径时
// 不会因上下文段里恰好出现 ".env" 之类而误报。
func appsHasParentAnchoredCredentialPair(path string) bool {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		switch parts[i-1] {
		case ".aws":
			if parts[i] == "credentials" {
				return true
			}
		case ".docker":
			if parts[i] == "config.json" {
				return true
			}
		case ".kube":
			if parts[i] == "config" {
				return true
			}
		}
	}
	return false
}

// appsIsSensitiveCandidate 是 html-publish 的调用点封装，两道扫描：
//  1. 用完整匹配器扫 RelPath（覆盖在树内的常见情况，如 ./site/.env）。
//  2. 在 rootPath 与 candidate 的边界处只用 parent-anchored 匹配器再探一次：
//     walker 通过 filepath.Rel 剥掉了根段，当 --path 本身就是约定父目录（如 ./.aws）
//     时 RelPath 退化成裸 "credentials"，第 1 步无父可锚。把根的 basename
//     （单文件形态再加根父目录 basename）重新前缀回去暴露缺失段。叶子匹配器
//     不在这步重跑，避免祖先里出现 ".env" 之类时把其下每个文件都误判。
func appsIsSensitiveCandidate(rootPath string, c appsCandidate) bool {
	if appsIsSensitiveRelPath(c.RelPath) {
		return true
	}
	for _, ctx := range []string{filepath.Base(rootPath), filepath.Base(filepath.Dir(rootPath))} {
		switch ctx {
		case "", ".", "..", "/":
			continue
		}
		if appsHasParentAnchoredCredentialPair(filepath.ToSlash(filepath.Join(ctx, c.RelPath))) {
			return true
		}
	}
	return false
}
