package cmd

import (
	"path/filepath"
	"strings"
)

// 凭证文件扫描——发布前检查产物目录中的敏感凭证文件。
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
//  1. 用完整匹配器扫 RelPath（覆盖在树内的常见情况，如 ./site/.env、嵌套的 .aws/credentials）。
//  2. 在 rootPath 与 candidate 的边界处只用 parent-anchored 匹配器再探一次，回填 walker 用
//     filepath.Rel 剥掉的、刚好缺失的那一段父目录上下文。这一步按 --path 形态选 **唯一** 上下文段：
//     - 目录形态（rootIsDir）：RelPath 已含根下完整相对路径，只需用根 basename 锚定
//     （--path ./.aws 下裸 "credentials" → ".aws/credentials"）。绝不能借祖父目录，
//     否则 --path ./.aws/sub 下的普通 "credentials" 会被误判成 .aws/credentials（误报）。
//     - 单文件形态：Base(rootPath) 是文件名本身、无锚定价值，改用根的父目录 basename
//     （--path ./.aws/credentials → Dir 为 ./.aws → ".aws/credentials"）。
//     叶子匹配器不在这步重跑，避免祖先里出现 ".env" 之类时把其下每个文件都误判。
func appsIsSensitiveCandidate(rootPath string, rootIsDir bool, c appsCandidate) bool {
	if appsIsSensitiveRelPath(c.RelPath) {
		return true
	}
	var ctx string
	if rootIsDir {
		ctx = filepath.Base(rootPath)
	} else {
		ctx = filepath.Base(filepath.Dir(rootPath))
	}
	switch ctx {
	case "", ".", "..", "/":
		return false
	}
	return appsHasParentAnchoredCredentialPair(filepath.ToSlash(filepath.Join(ctx, c.RelPath)))
}
