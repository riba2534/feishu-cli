package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/safefile"
)

// WhiteboardCLIBridgeAvailable 检测 whiteboard-cli 是否在 PATH 中可用
func WhiteboardCLIBridgeAvailable() bool {
	_, err := exec.LookPath("whiteboard-cli")
	return err == nil
}

// WhiteboardCLIBridgeVersion 返回 whiteboard-cli 的版本字符串（未安装返回 ""）
func WhiteboardCLIBridgeVersion() string {
	cmd := exec.Command("whiteboard-cli", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// RenderDiagramToOpenAPINodes 把 Mermaid/DSL/SVG 源码通过 whiteboard-cli 本地转换为
// 飞书 OpenAPI 期望的节点 JSON 数组（用于直接喂给 CreateBoardNodes）。
//
// 输入：
//
//	source    — 图表源码或本地文件路径
//	syntax    — "mermaid" / "svg" / "dsl"（dsl 由 whiteboard-cli 自动识别）
//	asFile    — true 表示 source 是文件路径，false 表示是字符串内容
//
// 返回：节点 JSON 数组的字符串形式，可直接传给 CreateBoardNodes。
func RenderDiagramToOpenAPINodes(source, syntax string, asFile bool) (string, error) {
	if !WhiteboardCLIBridgeAvailable() {
		return "", fmt.Errorf("whiteboard-cli 未安装，请运行 npm install -g @larksuite/whiteboard-cli")
	}

	// 准备输入文件（whiteboard-cli 通过 -i 读，stdin 不可靠）
	tmpDir, err := os.MkdirTemp("", "wb-bridge-")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var inputPath string
	switch {
	case asFile:
		// 交给外部 whiteboard-cli 读取的本地文件同样拒绝敏感目录
		if _, err := safefile.StatInputFile(source); err != nil {
			return "", err
		}
		inputPath = source
	default:
		ext := ".mmd"
		switch syntax {
		case "svg":
			ext = ".svg"
		case "dsl":
			ext = ".json"
		}
		inputPath = filepath.Join(tmpDir, "input"+ext)
		if err := os.WriteFile(inputPath, []byte(source), 0644); err != nil {
			return "", fmt.Errorf("写入临时输入文件失败: %w", err)
		}
	}

	outputPath := filepath.Join(tmpDir, "nodes.json")

	args := []string{"-i", inputPath, "-t", "openapi", "-o", outputPath}
	if syntax != "" && syntax != "dsl" {
		args = append(args, "-f", syntax)
	}

	cmd := exec.Command("whiteboard-cli", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whiteboard-cli 调用失败: %w\nstderr: %s", err, stderr.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("读取 whiteboard-cli 输出失败: %w", err)
	}

	// whiteboard-cli 的 openapi 模式输出可能是 {nodes: [...]} 包装结构，
	// 也可能直接是数组。两种情况都归一化为「JSON 数组」字符串。
	var asArray []json.RawMessage
	if err := json.Unmarshal(data, &asArray); err == nil {
		out, _ := json.Marshal(asArray)
		return string(out), nil
	}
	var asWrapper struct {
		Nodes []json.RawMessage `json:"nodes"`
		Data  struct {
			Nodes []json.RawMessage `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &asWrapper); err == nil {
		nodes := asWrapper.Nodes
		if len(nodes) == 0 {
			nodes = asWrapper.Data.Nodes
		}
		if len(nodes) > 0 {
			out, _ := json.Marshal(nodes)
			return string(out), nil
		}
	}
	return "", fmt.Errorf("无法识别 whiteboard-cli 输出格式（前 200 字节: %s）",
		safePreview(data, 200))
}

func safePreview(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// LinkLocalEngineWarning 给出"本地引擎已不可用"的标准化警告语
func LinkLocalEngineWarning() string {
	return fmt.Sprintf("[%s] whiteboard-cli 不可用，自动降级到飞书服务端引擎",
		time.Now().Format("15:04:05"))
}
