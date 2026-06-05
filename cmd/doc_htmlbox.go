package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	// defaultHtmlboxComponentTypeID 是飞书「妙笔BOX」HTML 小组件的组件类型 ID。
	// 该组件把整段 HTML 存进 add_ons.record，在飞书 iframe 沙箱里真实渲染（可跑 CSS/JS 动画、ECharts 等）。
	// 海外 Lark / 其他环境如组件 ID 不同，可用 --component-type-id 覆盖。
	defaultHtmlboxComponentTypeID = "blk_6900429af84180025ce76527"
	// blockTypeAddOns 是 AddOns（小组件）块的 block_type 值。
	blockTypeAddOns = 40
)

var docHtmlboxCmd = &cobra.Command{
	Use:   "htmlbox",
	Short: "飞书妙笔BOX（HTML 小组件块）增删改查",
	Long: `操作飞书文档里的「妙笔BOX」HTML 小组件块（block_type=40）。

妙笔BOX 把一整页 HTML 存进块的 add_ons.record，在飞书 iframe 沙箱里真实执行 CSS/JS——
这是飞书文档里能跑动画/可交互图表（ECharts、Three.js、CSS 动画、Canvas）的唯一载体，
与画板（board）不同：画板的 svg 节点会被服务端栅格化成静态图，不会动。

子命令:
  create   往文档插入一个妙笔BOX 块
  update   更新妙笔BOX 块的 HTML（飞书 API 不支持原地改，走先建后删、同位置重建，block_id 会变）
  get      读回妙笔BOX 块当前的 HTML
  delete   删除一个妙笔BOX 块

身份: create/update/get/delete 默认 Bot（App Token），适合操作 feishu-cli 创建的文档，无需登录；
操作他人或你在飞书里手动创建的文档时，传 --user-access-token / 设 FEISHU_USER_ACCESS_TOKEN 切到 User 身份
（注意：同一文档读写应使用同一身份，否则可能 forBidden）。`,
}

var docHtmlboxCreateCmd = &cobra.Command{
	Use:   "create <document_id>",
	Short: "往文档插入妙笔BOX HTML 小组件块",
	Long: `往飞书文档插入一个妙笔BOX HTML 小组件块。

HTML 输入（三选一）:
  --html '<...>'          直接传 HTML 字符串
  --html-file widget.html 从文件读取
  --html-file -           从 stdin 读取

示例:
  feishu-cli doc htmlbox create <doc_id> --html-file widget.html
  feishu-cli doc htmlbox create <doc_id> --html '<div style="animation:spin 2s infinite">…</div>'
  cat widget.html | feishu-cli doc htmlbox create <doc_id> --html-file -
  feishu-cli doc htmlbox create <doc_id> --html-file widget.html --index 0 --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runHtmlboxCreate,
}

var docHtmlboxUpdateCmd = &cobra.Command{
	Use:   "update <document_id> <block_id>",
	Short: "更新妙笔BOX 块的 HTML（先建后删，同位置重建）",
	Long: `更新一个已有妙笔BOX 块的 HTML 内容。

注意: 飞书 OpenAPI 不支持原地更新 HTML 小组件（PATCH add_ons 返回 invalid param），
因此本命令通过「在原位置新建 + 删除旧块」（先建后删，中途失败不丢数据）实现，新块的 block_id 会与原来不同（输出里会返回 new_block_id）。

HTML 输入与 create 相同（--html / --html-file / --html-file -）。

示例:
  feishu-cli doc htmlbox update <doc_id> <block_id> --html-file widget-v2.html`,
	Args: cobra.ExactArgs(2),
	RunE: runHtmlboxUpdate,
}

var docHtmlboxGetCmd = &cobra.Command{
	Use:   "get <document_id> <block_id>",
	Short: "读回妙笔BOX 块的 HTML",
	Long: `读取一个妙笔BOX 块当前的 HTML 内容。

默认输出结构化 JSON（含 html 字段），可用 --jq '.html' 提取；
加 --raw 直接把纯 HTML 打到 stdout（配合重定向存文件）。

示例:
  feishu-cli doc htmlbox get <doc_id> <block_id>
  feishu-cli doc htmlbox get <doc_id> <block_id> --raw > current.html
  feishu-cli doc htmlbox get <doc_id> <block_id> --jq '.html'`,
	Args: cobra.ExactArgs(2),
	RunE: runHtmlboxGet,
}

var docHtmlboxDeleteCmd = &cobra.Command{
	Use:   "delete <document_id> <block_id>",
	Short: "删除一个妙笔BOX 块",
	Long: `删除一个妙笔BOX 块。仅能删除 block_type=40 的妙笔BOX 块（防误删其他块）；
删除普通块请用 feishu-cli doc delete。

示例:
  feishu-cli doc htmlbox delete <doc_id> <block_id>
  feishu-cli doc htmlbox delete <doc_id> <block_id> --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: runHtmlboxDelete,
}

// loadHTMLInput 读取 --html / --html-file（- 表示 stdin）的 HTML 内容并做基本校验。
func loadHTMLInput(cmd *cobra.Command) (string, error) {
	htmlStr, _ := cmd.Flags().GetString("html")
	htmlFile, _ := cmd.Flags().GetString("html-file")
	if htmlStr != "" && htmlFile != "" {
		return "", fmt.Errorf("--html 和 --html-file 不能同时使用")
	}
	var content string
	switch {
	case htmlFile == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("从 stdin 读取 HTML 失败: %w", err)
		}
		content = string(data)
	case htmlFile != "":
		data, err := os.ReadFile(htmlFile)
		if err != nil {
			return "", fmt.Errorf("读取 HTML 文件失败: %w", err)
		}
		content = string(data)
	default:
		content = htmlStr
	}
	// 不 TrimSpace 原文，保证 get --raw 能逐字节还原写入内容；仅用 trim 副本做校验。
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("请通过 --html 或 --html-file 提供 HTML 内容（--html-file - 从 stdin 读取）")
	}
	if !strings.Contains(content, "<") {
		return "", fmt.Errorf("内容不是有效的 HTML（未找到 < 标签）")
	}
	return content, nil
}

// buildHtmlboxRecord 把 HTML 包成妙笔BOX 的 record JSON 字符串：{"html":"..."}。
// 用 json.Marshal 保证转义正确，不手拼字符串。
func buildHtmlboxRecord(html string) (string, error) {
	b, err := json.Marshal(map[string]string{"html": html})
	if err != nil {
		return "", fmt.Errorf("构造 record 失败: %w", err)
	}
	return string(b), nil
}

// buildHtmlboxBlock 构造一个妙笔BOX（AddOns）块。
func buildHtmlboxBlock(componentTypeID, record string) *larkdocx.Block {
	bt := blockTypeAddOns
	addOns := larkdocx.NewAddOnsBuilder().
		ComponentTypeId(componentTypeID).
		Record(record).
		Build()
	return &larkdocx.Block{
		BlockType: &bt,
		AddOns:    addOns,
	}
}

// extractHTMLFromRecord 从 add_ons.record（{"html":"..."}）里解出 html 字段。
func extractHTMLFromRecord(record string) string {
	if strings.TrimSpace(record) == "" {
		return ""
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(record), &rec); err != nil {
		return ""
	}
	if h, ok := rec["html"].(string); ok {
		return h
	}
	return ""
}

// locateHtmlboxBlock 校验目标块是妙笔BOX 块，并定位它在父块中的位置（parentID + index）。
func locateHtmlboxBlock(documentID, blockID, token string) (blk *larkdocx.Block, parentID string, index int, err error) {
	blk, err = client.GetBlock(documentID, blockID, token)
	if err != nil {
		return nil, "", -1, err
	}
	bt := 0
	if blk.BlockType != nil {
		bt = *blk.BlockType
	}
	if bt != blockTypeAddOns {
		return nil, "", -1, fmt.Errorf("块 %s 不是妙笔BOX（HTML 小组件）块（block_type=%d，期望 %d）；删除其他块请用 feishu-cli doc delete", blockID, bt, blockTypeAddOns)
	}
	parentID = documentID
	if blk.ParentId != nil && *blk.ParentId != "" {
		parentID = *blk.ParentId
	}
	parent, perr := client.GetBlock(documentID, parentID, token)
	if perr != nil {
		return nil, "", -1, fmt.Errorf("获取父块 %s 失败: %w", parentID, perr)
	}
	index = -1
	for i, childID := range parent.Children {
		if childID == blockID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, "", -1, fmt.Errorf("在父块 %s 的子块列表中未找到 %s", parentID, blockID)
	}
	return blk, parentID, index, nil
}

func runHtmlboxCreate(cmd *cobra.Command, args []string) error {
	documentID := args[0]
	componentTypeID, _ := cmd.Flags().GetString("component-type-id")
	parentID, _ := cmd.Flags().GetString("parent-id")
	index, _ := cmd.Flags().GetInt("index")

	html, err := loadHTMLInput(cmd)
	if err != nil {
		return err
	}
	record, err := buildHtmlboxRecord(html)
	if err != nil {
		return err
	}
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	if parentID == "" {
		parentID = documentID
	}

	if o.DryRun {
		return output.Render(o, map[string]any{
			"action":            "create",
			"document_id":       documentID,
			"parent_id":         parentID,
			"index":             index,
			"component_type_id": componentTypeID,
			"html_len":          len(html),
			"dry_run":           true,
		})
	}

	if err := config.Validate(); err != nil {
		return err
	}
	token := resolveOptionalUserToken(cmd)

	block := buildHtmlboxBlock(componentTypeID, record)
	created, _, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{block}, index, token)
	if err != nil {
		return err
	}
	if len(created) == 0 {
		return fmt.Errorf("创建妙笔BOX 块失败：未返回块信息")
	}
	blockID := client.StringVal(created[0].BlockId)
	return output.Render(o, map[string]any{
		"action":            "create",
		"block_id":          blockID,
		"document_id":       documentID,
		"parent_id":         parentID,
		"component_type_id": componentTypeID,
		"html_len":          len(html),
	})
}

func runHtmlboxUpdate(cmd *cobra.Command, args []string) error {
	documentID := args[0]
	blockID := args[1]

	html, err := loadHTMLInput(cmd)
	if err != nil {
		return err
	}
	record, err := buildHtmlboxRecord(html)
	if err != nil {
		return err
	}
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}

	if o.DryRun {
		return output.Render(o, map[string]any{
			"action":      "update",
			"document_id": documentID,
			"block_id":    blockID,
			"html_len":    len(html),
			"strategy":    "create-new-then-delete-old",
			"dry_run":     true,
		})
	}

	if err := config.Validate(); err != nil {
		return err
	}
	token := resolveOptionalUserToken(cmd)

	// 飞书 API 不支持原地更新 add_ons（PATCH 返回 1770001 invalid param），
	// 因此走「原位置新建 + 删除旧块」（先建后删，避免中途失败丢数据）。
	oldBlk, parentID, index, err := locateHtmlboxBlock(documentID, blockID, token)
	if err != nil {
		return err
	}

	// 默认沿用原块的 component_type_id，除非用户显式指定。
	componentTypeID, _ := cmd.Flags().GetString("component-type-id")
	if !cmd.Flags().Changed("component-type-id") && oldBlk.AddOns != nil && oldBlk.AddOns.ComponentTypeId != nil && *oldBlk.AddOns.ComponentTypeId != "" {
		componentTypeID = *oldBlk.AddOns.ComponentTypeId
	}

	// 先在原位置插入新块（成功后再删旧块），避免中途失败导致数据丢失。
	// 插入到 index 后，原块被后移到 index+1。
	block := buildHtmlboxBlock(componentTypeID, record)
	created, _, err := client.CreateBlock(documentID, parentID, []*larkdocx.Block{block}, index, token)
	if err != nil {
		return fmt.Errorf("创建新块失败（原块未改动）: %w", err)
	}
	if len(created) == 0 {
		return fmt.Errorf("创建新块失败：未返回块信息（原块 %s 未改动）", blockID)
	}
	newBlockID := client.StringVal(created[0].BlockId)
	// 删除被后移到 index+1 的旧块。
	if _, err := client.DeleteBlocks(documentID, parentID, index+1, index+2, token); err != nil {
		return fmt.Errorf("新块已创建（%s），但删除原块 %s 失败，请手动删除原块: %w", newBlockID, blockID, err)
	}
	return output.Render(o, map[string]any{
		"action":            "update",
		"old_block_id":      blockID,
		"new_block_id":      newBlockID,
		"document_id":       documentID,
		"parent_id":         parentID,
		"index":             index,
		"component_type_id": componentTypeID,
		"html_len":          len(html),
		"note":              "飞书 API 不支持原地更新 HTML 组件，已通过原位置新建+删除旧块实现，block_id 已变化",
	})
}

func runHtmlboxGet(cmd *cobra.Command, args []string) error {
	documentID := args[0]
	blockID := args[1]
	raw, _ := cmd.Flags().GetBool("raw")

	if err := config.Validate(); err != nil {
		return err
	}
	token := resolveOptionalUserToken(cmd)

	blk, err := client.GetBlock(documentID, blockID, token)
	if err != nil {
		return err
	}
	bt := 0
	if blk.BlockType != nil {
		bt = *blk.BlockType
	}
	if bt != blockTypeAddOns {
		return fmt.Errorf("块 %s 不是妙笔BOX（HTML 小组件）块（block_type=%d，期望 %d）", blockID, bt, blockTypeAddOns)
	}

	record := ""
	componentTypeID := ""
	if blk.AddOns != nil {
		if blk.AddOns.Record != nil {
			record = *blk.AddOns.Record
		}
		if blk.AddOns.ComponentTypeId != nil {
			componentTypeID = *blk.AddOns.ComponentTypeId
		}
	}
	html := extractHTMLFromRecord(record)

	if raw {
		if html == "" {
			fmt.Fprintln(os.Stderr, "警告: 未从该块解析出 HTML（record 为空或无 html 字段），输出为空")
		}
		fmt.Print(html)
		return nil
	}

	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	return output.Render(o, map[string]any{
		"block_id":          blockID,
		"document_id":       documentID,
		"component_type_id": componentTypeID,
		"html":              html,
		"html_len":          len(html),
	})
}

func runHtmlboxDelete(cmd *cobra.Command, args []string) error {
	documentID := args[0]
	blockID := args[1]

	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	if o.DryRun {
		return output.Render(o, map[string]any{
			"action":      "delete",
			"document_id": documentID,
			"block_id":    blockID,
			"dry_run":     true,
		})
	}

	if err := config.Validate(); err != nil {
		return err
	}
	token := resolveOptionalUserToken(cmd)

	_, parentID, index, err := locateHtmlboxBlock(documentID, blockID, token)
	if err != nil {
		return err
	}
	if _, err := client.DeleteBlocks(documentID, parentID, index, index+1, token); err != nil {
		return err
	}
	return output.Render(o, map[string]any{
		"action":           "delete",
		"deleted_block_id": blockID,
		"document_id":      documentID,
		"parent_id":        parentID,
		"index":            index,
	})
}

func init() {
	docCmd.AddCommand(docHtmlboxCmd)
	docHtmlboxCmd.AddCommand(docHtmlboxCreateCmd)
	docHtmlboxCmd.AddCommand(docHtmlboxUpdateCmd)
	docHtmlboxCmd.AddCommand(docHtmlboxGetCmd)
	docHtmlboxCmd.AddCommand(docHtmlboxDeleteCmd)

	// create
	docHtmlboxCreateCmd.Flags().String("html", "", "HTML 字符串")
	docHtmlboxCreateCmd.Flags().String("html-file", "", "HTML 文件路径（- 表示 stdin）")
	docHtmlboxCreateCmd.Flags().String("component-type-id", defaultHtmlboxComponentTypeID, "妙笔BOX 组件类型 ID")
	docHtmlboxCreateCmd.Flags().String("parent-id", "", "父块 ID（默认: 文档根节点）")
	docHtmlboxCreateCmd.Flags().Int("index", -1, "插入位置索引（-1 表示末尾）")
	docHtmlboxCreateCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddOutputFlags(docHtmlboxCreateCmd)
	output.AddDryRunFlag(docHtmlboxCreateCmd)

	// update
	docHtmlboxUpdateCmd.Flags().String("html", "", "HTML 字符串")
	docHtmlboxUpdateCmd.Flags().String("html-file", "", "HTML 文件路径（- 表示 stdin）")
	docHtmlboxUpdateCmd.Flags().String("component-type-id", defaultHtmlboxComponentTypeID, "妙笔BOX 组件类型 ID（默认沿用原块）")
	docHtmlboxUpdateCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddOutputFlags(docHtmlboxUpdateCmd)
	output.AddDryRunFlag(docHtmlboxUpdateCmd)

	// get
	docHtmlboxGetCmd.Flags().Bool("raw", false, "直接输出纯 HTML 到 stdout（配合 > file.html）")
	docHtmlboxGetCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddFormatFlags(docHtmlboxGetCmd)

	// delete
	docHtmlboxDeleteCmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddOutputFlags(docHtmlboxDeleteCmd)
	output.AddDryRunFlag(docHtmlboxDeleteCmd)
}
