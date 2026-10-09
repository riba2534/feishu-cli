package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

var sheetCellRE = regexp.MustCompile(`^[A-Za-z]+[1-9][0-9]*$`)

var sheetImageWriteBatchCmd = &cobra.Command{
	Use:     "write-batch <spreadsheet_token|url> <sheet_id>",
	Aliases: []string{"batch-write"},
	Short:   "批量下载并写入原生单元格图片",
	Long: `从 JSON manifest 批量写入原生图片单元格，并通过 V3 read-rich 回读验证。

manifest 可以是文件路径、行内 JSON 数组或 "-"（从标准输入读取）。
每项必须包含 cell，且 url/path 二选一（name 可选）：
  [
    {"cell":"B2","url":"https://example.com/1.jpg"},
    {"cell":"B3","path":"/tmp/2.png","name":"product-2.png"}
  ]

网络图片仅接受 HTTPS，下载后校验响应状态、图片格式和大小。
下载与预处理可并发，同一批次的图片写入串行执行。
BMP/TIFF/WebP 自动转 PNG，原文件不变；HEIC/BPG 原样提交，结果取决于服务端支持。
文件名缺少有效图片后缀时按实际格式补齐，转码图片统一使用 .png 后缀。
成功条件是每个目标单元格回读为 type=image 且包含 image_token。部分失败会输出逐格结果并返回非零退出码。

示例:
  feishu-cli sheet image write-batch shtcnxxxxxx 0b1212 --manifest images.json
  cat images.json | feishu-cli sheet image write-batch shtcnxxxxxx 0b1212 --manifest - -o json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest, _ := cmd.Flags().GetString("manifest")
		workers, _ := cmd.Flags().GetInt("workers")
		maxBytes, _ := cmd.Flags().GetInt64("max-image-bytes")
		allowPrivate, _ := cmd.Flags().GetBool("allow-private-net")
		output, _ := cmd.Flags().GetString("output")
		if output != "" && output != "text" && output != "json" {
			return fmt.Errorf("不支持的输出格式 %q，仅支持 text, json", output)
		}

		items, err := readSheetImageBatchManifest(manifest, args[1], cmd.InOrStdin())
		if err != nil {
			return err
		}

		target, err := newSheetTarget(cmd, args[0])
		if err != nil {
			return err
		}
		opts := client.BatchWriteSheetImageOptions{
			Workers:         workers,
			MaxBytes:        maxBytes,
			AllowPrivateNet: allowPrivate,
		}

		result, runErr := client.BatchWriteSheetImages(cmd.Context(), target.Token, args[1], items, opts, target.UAT)
		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "批量图片写入完成：总计 %d，成功 %d，失败 %d，回读验证 %t\n", result.Total, result.Written, result.Failed, result.Verified)
			for _, item := range result.Outcomes {
				if item.Status == "failed" {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", item.Cell, item.Error)
				}
			}
		}
		return runErr
	},
}

func readSheetImageBatchManifest(manifestStr, sheetID string, in io.Reader) ([]client.BatchWriteSheetImageItem, error) {
	text := strings.TrimSpace(manifestStr)
	if text == "" {
		return nil, fmt.Errorf("--manifest 为必填项（可传文件路径、行内 JSON 数组或 - 从标准输入读取）")
	}

	var raw []byte
	var err error
	if text == "-" {
		raw, err = io.ReadAll(in)
		if err != nil {
			return nil, fmt.Errorf("读取标准输入失败: %w", err)
		}
	} else if _, statErr := os.Stat(text); statErr == nil {
		// 优先：若路径在磁盘上真实存在，直接读取文件（兼容带 [ 前缀的文件路径）
		raw, err = readLocalInputFile(text)
		if err != nil {
			return nil, fmt.Errorf("读取 manifest 文件失败: %w", err)
		}
	} else if strings.HasPrefix(text, "[") {
		raw = []byte(text)
	} else if strings.HasPrefix(text, "{") {
		return nil, fmt.Errorf("manifest 必须是 JSON 数组（形如 [{...}]），不支持单个 JSON 对象")
	} else {
		return nil, fmt.Errorf("读取 manifest 失败（文件不存在或不是合法 JSON 数组）: %s", text)
	}

	var items []client.BatchWriteSheetImageItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("解析 manifest 失败（需要 JSON 数组）: %w", err)
	}
	if len(items) == 0 || len(items) > client.MaxSheetImageBatchItems {
		return nil, fmt.Errorf("manifest 图片数量必须为 1-%d（当前 %d）", client.MaxSheetImageBatchItems, len(items))
	}

	seen := map[string]struct{}{}
	for i := range items {
		cell, err := normalizeSheetImageBatchCell(unescapeSheetRange(items[i].Cell), sheetID)
		if err != nil {
			return nil, fmt.Errorf("manifest 第 %d 项: %w", i+1, err)
		}
		items[i].Cell = cell

		items[i].URL = strings.TrimSpace(items[i].URL)
		items[i].Path = strings.TrimSpace(items[i].Path)
		items[i].Name = strings.TrimSpace(items[i].Name)
		hasURL := items[i].URL != ""
		hasPath := items[i].Path != ""
		if hasURL == hasPath {
			return nil, fmt.Errorf("manifest 第 %d 项必须且只能设置 url/path 之一", i+1)
		}
		if _, ok := seen[cell]; ok {
			return nil, fmt.Errorf("manifest 包含重复单元格 %s", cell)
		}
		seen[cell] = struct{}{}
	}
	return items, nil
}

func normalizeSheetImageBatchCell(value, sheetID string) (string, error) {
	text := strings.TrimSpace(value)
	if idx := strings.LastIndex(text, "!"); idx >= 0 {
		prefix := strings.Trim(text[:idx], "'")
		if prefix != sheetID {
			return "", fmt.Errorf("cell 的 sheet_id %q 与目标 %q 不一致（飞书接口要求使用 sheet_id 而非工作表标题）", prefix, sheetID)
		}
		text = text[idx+1:]
	}

	if idx := strings.Index(text, ":"); idx >= 0 {
		start := text[:idx]
		end := text[idx+1:]
		if !strings.EqualFold(start, end) {
			return "", fmt.Errorf("cell 只支持单个单元格，起止不能不同: %q", value)
		}
		text = start
	}

	if !sheetCellRE.MatchString(text) {
		return "", fmt.Errorf("cell 必须是单个 A1 地址（当前 %q）", value)
	}
	text = strings.ToUpper(text)
	return fmt.Sprintf("%s!%s:%s", sheetID, text, text), nil
}

func init() {
	sheetImageCmd.AddCommand(sheetImageWriteBatchCmd)

	sheetImageWriteBatchCmd.Flags().String("manifest", "", "图片 manifest（JSON 数组、文件路径或 - 从标准输入读取，必填）")
	sheetImageWriteBatchCmd.Flags().Int("workers", client.DefaultSheetImageBatchWorkers, "并发下载/预处理 worker 数（1-10，图片写入串行执行）")
	sheetImageWriteBatchCmd.Flags().Int64("max-image-bytes", client.DefaultSheetImageBatchMaxBytes, "单张图片最大字节数")
	sheetImageWriteBatchCmd.Flags().Bool("allow-private-net", false, "允许从私有网络或内网 IP 下载图片（用于企业内网 CDN/对象存储）")
	sheetImageWriteBatchCmd.Flags().StringP("output", "o", "text", "输出格式: text, json")
	sheetImageWriteBatchCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问无 App 权限的表格）")
	mustMarkFlagRequired(sheetImageWriteBatchCmd, "manifest")
}
