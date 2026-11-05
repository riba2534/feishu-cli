package cmd

import (
	"fmt"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"path/filepath"
	"strings"
)

const (
	driveImport20MB  int64 = 20 * 1024 * 1024
	driveImport100MB int64 = 100 * 1024 * 1024
	driveImport500MB int64 = 500 * 1024 * 1024
	driveImport600MB int64 = 600 * 1024 * 1024
	driveImport800MB int64 = 800 * 1024 * 1024
)

// 官方扩展名 → 可导入的目标类型矩阵。
var driveImportExtToDocTypes = map[string][]string{
	"docx":     {"docx"},
	"doc":      {"docx"},
	"txt":      {"docx"},
	"md":       {"docx"},
	"mark":     {"docx"},
	"markdown": {"docx"},
	"html":     {"docx"},
	"xlsx":     {"sheet", "bitable"},
	"xls":      {"sheet"},
	"csv":      {"sheet", "bitable"},
	"base":     {"bitable"},
	"pptx":     {"slides"},
}

var driveImportAllowedTypes = []string{"docx", "sheet", "bitable", "slides"}

func driveImportFileExtension(filePath, effectiveExt string) string {
	if strings.TrimSpace(effectiveExt) != "" {
		return strings.TrimPrefix(strings.ToLower(effectiveExt), ".")
	}
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
}

func driveImportFileSizeLimit(ext, docType string) (int64, bool) {
	switch ext {
	case "docx", "doc":
		return driveImport600MB, true
	case "pptx":
		return driveImport500MB, true
	case "txt", "md", "mark", "markdown", "html", "xls", "base":
		return driveImport20MB, true
	case "xlsx":
		return driveImport800MB, true
	case "csv":
		if docType == "bitable" {
			return driveImport100MB, true
		}
		return driveImport20MB, true
	default:
		return 0, false
	}
}

func validateDriveImportFileSize(ext, docType string, fileSize int64) error {
	limit, ok := driveImportFileSizeLimit(ext, docType)
	if !ok || fileSize <= limit {
		return nil
	}
	if ext == "csv" {
		return fmt.Errorf("文件大小 %s 超过 .csv 导入为 %s 的限制 %s", formatByteSize(fileSize), docType, formatByteSize(limit))
	}
	return fmt.Errorf("文件大小 %s 超过 .%s 导入限制 %s", formatByteSize(fileSize), ext, formatByteSize(limit))
}

func validateDriveImportSpec(filePath, docType, folderToken, targetToken, effectiveExt string) error {
	ext := driveImportFileExtension(filePath, effectiveExt)
	if ext == "" {
		return fmt.Errorf("文件必须带扩展名（如 .md / .docx / .xlsx / .pptx）")
	}
	if err := validateEnum(docType, "--type", driveImportAllowedTypes); err != nil {
		return err
	}
	supported, ok := driveImportExtToDocTypes[ext]
	if !ok {
		return fmt.Errorf("不支持的文件扩展名: .%s。支持: docx, doc, txt, md, mark, markdown, html, xlsx, xls, csv, base, pptx", ext)
	}
	typeAllowed := false
	for _, allowed := range supported {
		if allowed == docType {
			typeAllowed = true
			break
		}
	}
	if !typeAllowed {
		return fmt.Errorf("文件类型不匹配: .%s 不能导入为 %s（允许: %s）", ext, docType, strings.Join(supported, ", "))
	}
	if strings.TrimSpace(targetToken) != "" && docType != "bitable" {
		return clierr.Usagef("--target-token 仅在 --type bitable 时可用")
	}
	_ = folderToken
	return nil
}

func driveImportDefaultFileName(filePath, explicitName string) string {
	if strings.TrimSpace(explicitName) != "" {
		return explicitName
	}
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	if ext == "" {
		return base
	}
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return base
	}
	return name
}

func driveImportSourceFileName(filePath, effectiveExt string) string {
	base := filepath.Base(filePath)
	raw := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if effectiveExt != "" && effectiveExt != raw {
		return strings.TrimSuffix(base, filepath.Ext(base)) + "." + effectiveExt
	}
	return base
}

func formatByteSize(n int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
