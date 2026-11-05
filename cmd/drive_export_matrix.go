package cmd

import (
	"fmt"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"path/filepath"
	"strings"
)

const (
	driveExportResolvedDocTypeValues = "doc, docx, sheet, bitable, slides"
	driveExportInputDocTypeValues    = driveExportResolvedDocTypeValues + ", wiki"
	driveExportFileExtensionValues   = "docx, pdf, xlsx, csv, markdown, base, pptx"
)

func driveExportAllowedFileExtensions(docType string) []string {
	switch normalizeDriveExportDocType(docType) {
	case "doc":
		return []string{"docx", "pdf"}
	case "docx":
		return []string{"docx", "pdf", "markdown"}
	case "sheet":
		return []string{"xlsx", "csv"}
	case "bitable":
		return []string{"xlsx", "csv", "base"}
	case "slides":
		return []string{"pptx", "pdf"}
	default:
		return []string{"docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx"}
	}
}

func normalizeDriveExportDocType(docType string) string {
	switch strings.ToLower(strings.TrimSpace(docType)) {
	case "base":
		return "bitable"
	default:
		return strings.ToLower(strings.TrimSpace(docType))
	}
}

func isDriveExportDocType(docType string) bool {
	switch normalizeDriveExportDocType(docType) {
	case "doc", "docx", "sheet", "bitable", "slides":
		return true
	default:
		return false
	}
}

func driveExportFileExtensionAllowed(docType, fileExtension string) bool {
	for _, allowed := range driveExportAllowedFileExtensions(docType) {
		if fileExtension == allowed {
			return true
		}
	}
	return false
}

func validateDriveExportFormat(docType, fileExtension, subID string, onlySchema bool) error {
	switch fileExtension {
	case "docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx":
	default:
		return fmt.Errorf("不支持的 --file-extension %q，允许: %s", fileExtension, driveExportFileExtensionValues)
	}
	if !isDriveExportDocType(docType) {
		return fmt.Errorf("不支持的 --doc-type %q，允许: %s", docType, driveExportInputDocTypeValues)
	}
	if !driveExportFileExtensionAllowed(docType, fileExtension) {
		return fmt.Errorf("不支持的导出组合: --doc-type %s 不能导出为 %s（允许: %s）",
			docType, fileExtension, strings.Join(driveExportAllowedFileExtensions(docType), ", "))
	}
	if onlySchema && (docType != "bitable" || fileExtension != "base") {
		return clierr.Usagef("--only-schema 仅在导出 bitable 为 base 时使用")
	}
	if strings.TrimSpace(subID) != "" {
		if fileExtension != "csv" || (docType != "sheet" && docType != "bitable") {
			return clierr.Usagef("--sub-id 仅在 sheet/bitable 导出 csv 时使用")
		}
	}
	if fileExtension == "csv" && (docType == "sheet" || docType == "bitable") && strings.TrimSpace(subID) == "" {
		return fmt.Errorf("导出 sheet/bitable 为 csv 时 --sub-id 必填")
	}
	return nil
}

func exportFileSuffix(fileExtension string) string {
	switch fileExtension {
	case "markdown":
		return ".md"
	case "docx", "pdf", "xlsx", "csv", "base", "pptx":
		return "." + fileExtension
	default:
		return ""
	}
}

func ensureExportFileExtension(name, fileExtension string) string {
	expected := exportFileSuffix(fileExtension)
	if expected == "" {
		return name
	}
	if strings.EqualFold(filepath.Ext(name), expected) {
		return name
	}
	return name + expected
}

func mapExtensionToSuffix(ext string) string {
	switch ext {
	case "markdown":
		return "md"
	case "docx", "pdf", "xlsx", "csv", "base", "pptx":
		return ext
	default:
		return ext
	}
}
