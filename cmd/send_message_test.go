package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -------------------------------------------------------------------
// isLocalPath tests - 纯函数，无 API 调用
// -------------------------------------------------------------------

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// 远程 URL 应该返回 false
		{"http URL", "http://example.com/image.png", false},
		{"https URL", "https://example.com/image.png", false},
		{"https URL 带参数", "https://example.com/image.png?token=abc", false},

		// 飞书 image_key 应该返回 false
		{"img_ 开头", "img_v2_abc123", false},
		{"file_v2_ 开头", "file_v2_abc123", false},

		// 图片扩展名应该是本地路径
		{"相对路径 png", "screenshot.png", true},
		{"相对路径 jpg", "photo.jpg", true},
		{"相对路径 jpeg", "image.jpeg", true},
		{"相对路径 gif", "animation.gif", true},
		{"相对路径 bmp", "icon.bmp", true},
		{"相对路径 webp", "image.webp", true},
		{"相对路径 svg（IM 不支持）", "vector.svg", false},

		// 绝对路径应该是本地路径
		{"绝对路径 Unix", "/Users/test/image.png", true},
		{"绝对路径 Windows", "C:\\Users\\test\\image.png", true},
		{"绝对路径 带扩展名", "/home/user/docs/photo.JPG", true},

		// 相对路径带目录分隔符
		{"相对路径带斜杠", "images/logo.png", true},
		{"相对路径反斜杠", "images\\logo.png", true},
		{"上级目录", "../assets/icon.png", true},
		{"当前目录", "./screenshot.png", true},
		{"业务 key 含斜杠但无图片扩展名", "tenant/image/key", false},

		// 非图片扩展名不应该被识别为本地图片路径
		{"txt 文件（无分隔符）", "readme.txt", false},
		{"md 文件（无分隔符）", "document.md", false},
		{"空字符串", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLocalPath(tt.path, ".")
			if result != tt.expected {
				t.Errorf("isLocalPath(%q) = %v, 期望 %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsLocalPathPrefersExistingPrefixedFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "img_logo.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !isLocalPath("img_logo.png", tempDir) {
		t.Fatal("存在的 img_ 前缀相对文件应优先识别为本地路径")
	}
	if isLocalPath("img_missing.png", tempDir) {
		t.Fatal("不存在的 img_ 前缀值应继续识别为飞书资源 key")
	}
}

// -------------------------------------------------------------------
// processJSONLocalImages 测试 - 纯逻辑，不触发实际上传
// 覆盖跳过远程/已上传图片，以及本地图片失败时阻断发送。
// -------------------------------------------------------------------

func TestProcessJSONLocalImages_SkipRemoteImages(t *testing.T) {
	// 测试远程图片 URL 应该被跳过（不会尝试上传）
	tests := []struct {
		name        string
		jsonContent string
		expectKey   string // 期望在结果中保留原值
	}{
		{
			name: "https URL",
			jsonContent: `{
				"tag": "img",
				"image_key": "https://example.com/image.png"
			}`,
			expectKey: "https://example.com/image.png",
		},
		{
			name: "http URL",
			jsonContent: `{
				"tag": "img",
				"image_key": "http://cdn.example.com/photo.jpg"
			}`,
			expectKey: "http://cdn.example.com/photo.jpg",
		},
		{
			name: "img_ 前缀（已上传）",
			jsonContent: `{
				"tag": "img",
				"image_key": "img_v2_already_uploaded"
			}`,
			expectKey: "img_v2_already_uploaded",
		},
		{
			name: "file_ 前缀（已上传文件）",
			jsonContent: `{
				"tag": "img",
				"image_key": "file_v2_xxx"
			}`,
			expectKey: "file_v2_xxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data interface{}
			if err := json.Unmarshal([]byte(tt.jsonContent), &data); err != nil {
				t.Fatalf("JSON 解析失败: %v", err)
			}

			changed, newData, count, processErr := processJSONLocalImages(data, "/tmp/nonexistent")
			if processErr != nil {
				t.Fatalf("远程/已上传图片不应触发错误: %v", processErr)
			}

			// 远程图片和已上传图片不应该被修改
			if changed {
				t.Errorf("远程/已上传图片不应该被修改，changed=true")
			}
			if count != 0 {
				t.Errorf("上传计数应该为 0，实际 %d", count)
			}

			// 验证结果保留原值
			resultBytes, err := json.Marshal(newData)
			if err != nil {
				t.Fatalf("JSON 序列化失败: %v", err)
			}
			if !strings.Contains(string(resultBytes), tt.expectKey) {
				t.Errorf("结果 JSON 不包含期望的 image_key %q，结果: %s", tt.expectKey, string(resultBytes))
			}
		})
	}
}

func TestProcessJSONLocalImages_RejectsNonExistentFiles(t *testing.T) {
	// 显式要求上传时，本地图片缺失必须阻止后续发送。
	jsonContent := `{
		"tag": "img",
		"image_key": "/nonexistent/path/to/image.png"
	}`

	var data interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	changed, _, count, err := processJSONLocalImages(data, "/tmp")

	// 不存在的文件不应该被修改
	if changed {
		t.Errorf("不存在的文件不应该被修改，changed=true")
	}
	if count != 0 {
		t.Errorf("上传计数应该为 0，实际 %d", count)
	}

	if err == nil || !strings.Contains(err.Error(), "本地图片不可用") {
		t.Fatalf("期望本地图片缺失错误，实际: %v", err)
	}
}

func TestProcessJSONLocalImages_NonImageTag(t *testing.T) {
	// 测试非 img 标签不应该被修改
	jsonContent := `{
		"tag": "text",
		"image_key": "some.png"
	}`

	var data interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	changed, newData, count, err := processJSONLocalImages(data, "/tmp")
	if err != nil {
		t.Fatalf("非 img 标签不应触发错误: %v", err)
	}

	if changed {
		t.Errorf("非 img 标签不应该被修改")
	}
	if count != 0 {
		t.Errorf("上传计数应该为 0")
	}

	resultBytes, _ := json.Marshal(newData)
	if !strings.Contains(string(resultBytes), "some.png") {
		t.Errorf("结果应该保留原值，结果: %s", string(resultBytes))
	}
}

func TestProcessJSONLocalImages_InvalidJSON(t *testing.T) {
	// 非 JSON 字符串应该原样返回
	changed, result, count, err := processJSONLocalImages("not json", "/tmp")
	if err != nil {
		t.Fatalf("标量值不应触发错误: %v", err)
	}
	if changed {
		t.Errorf("非 JSON 字符串不应该被修改")
	}
	if result != "not json" {
		t.Errorf("非 JSON 字符串应该原样返回")
	}
	if count != 0 {
		t.Errorf("计数应该为 0")
	}
}

func TestProcessJSONLocalImages_NestedStructure(t *testing.T) {
	// 测试嵌套 JSON 结构中的 img 标签
	jsonContent := `{
		"header": {
			"template": "blue",
			"title": {"tag": "plain_text", "content": "测试"}
		},
		"elements": [
			{"tag": "markdown", "content": "hello"},
			{"tag": "img", "image_key": "https://example.com/remote.png"}
		]
	}`

	var data interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	changed, _, count, err := processJSONLocalImages(data, "/tmp")
	if err != nil {
		t.Fatalf("远程图片不应触发错误: %v", err)
	}

	// 远程图片不应该被处理
	if changed {
		t.Errorf("远程图片不应该被处理")
	}
	if count != 0 {
		t.Errorf("计数应该为 0")
	}
}

func TestProcessJSONLocalImages_ArrayStructure(t *testing.T) {
	// 测试数组结构
	jsonContent := `[
		{"tag": "img", "image_key": "https://example.com/a.png"},
		{"tag": "text", "text": "hello"},
		{"tag": "img", "image_key": "img_v2_b"}
	]`

	var data interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	changed, _, count, err := processJSONLocalImages(data, "/tmp")
	if err != nil {
		t.Fatalf("远程/已上传图片不应触发错误: %v", err)
	}

	// 所有都是远程/已上传图片，不应该被处理
	if changed {
		t.Errorf("远程/已上传图片不应该被处理")
	}
	if count != 0 {
		t.Errorf("计数应该为 0")
	}
}

func TestProcessJSONLocalImages_UploadsCardV2LocalAssets(t *testing.T) {
	tempDir := t.TempDir()
	for _, name := range []string{"cover.png", "icon.png"} {
		if err := os.WriteFile(
			filepath.Join(tempDir, name),
			[]byte("\x89PNG\r\n\x1a\n"),
			0o600,
		); err != nil {
			t.Fatalf("创建测试图片失败: %v", err)
		}
	}

	originalUpload := uploadIMImage
	uploadIMImage = func(path, _ string) (string, error) {
		return "img_v2_uploaded_" + filepath.Base(path), nil
	}
	defer func() { uploadIMImage = originalUpload }()

	jsonContent := `{
		"body": {
			"elements": [
				{"tag": "img", "img_key": "cover.png"},
				{
					"tag": "img_combination",
					"img_list": [
						{"img_key": "icon.png"},
						{"img_key": "img_v2_existing"}
					]
				}
			]
		}
	}`
	var data interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	changed, result, count, err := processJSONLocalImages(data, tempDir)
	if err != nil {
		t.Fatalf("上传本地素材失败: %v", err)
	}
	if !changed {
		t.Fatal("Card V2 本地图片应该被替换")
	}
	if count != 2 {
		t.Fatalf("上传计数 = %d，期望 2", count)
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	resultJSON := string(resultBytes)
	for _, expected := range []string{
		`"img_key":"img_v2_uploaded_cover.png"`,
		`"img_key":"img_v2_uploaded_icon.png"`,
		`"img_key":"img_v2_existing"`,
	} {
		if !strings.Contains(resultJSON, expected) {
			t.Errorf("结果缺少 %s：%s", expected, resultJSON)
		}
	}
}

func TestProcessJSONLocalImages_RejectsInvalidImageContent(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "secret.png"), []byte("not an image"), 0o600); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	var data interface{}
	if err := json.Unmarshal([]byte(`{"tag":"img","img_key":"secret.png"}`), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	changed, _, count, err := processJSONLocalImages(data, tempDir)
	if changed || count != 0 {
		t.Fatalf("非法图片不应被上传：changed=%v count=%d", changed, count)
	}
	if err == nil || !strings.Contains(err.Error(), "文件内容不是支持的图片格式") {
		t.Fatalf("期望图片内容错误，实际: %v", err)
	}
}

func TestProcessJSONLocalImages_StopsWhenUploadFails(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "icon.png"),
		[]byte("\x89PNG\r\n\x1a\n"),
		0o600,
	); err != nil {
		t.Fatalf("创建测试图片失败: %v", err)
	}
	originalUpload := uploadIMImage
	uploadIMImage = func(_, _ string) (string, error) {
		return "", os.ErrPermission
	}
	defer func() { uploadIMImage = originalUpload }()

	var data interface{}
	if err := json.Unmarshal([]byte(`{"tag":"img","img_key":"icon.png"}`), &data); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	changed, _, count, err := processJSONLocalImages(data, tempDir)
	if changed || count != 0 {
		t.Fatalf("上传失败不应产生替换：changed=%v count=%d", changed, count)
	}
	if err == nil || !strings.Contains(err.Error(), "上传图片") {
		t.Fatalf("期望上传错误，实际: %v", err)
	}
}

// -------------------------------------------------------------------
// processAndUploadLocalImages 测试 - 纯逻辑，不触发实际上传
// -------------------------------------------------------------------

func TestProcessAndUploadLocalImages_SkipRemoteMarkdownImages(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectContains string
	}{
		{
			name:           "https 图片 URL 保留",
			content:        "![](https://example.com/image.png)",
			expectContains: "https://example.com/image.png",
		},
		{
			name:           "http 图片 URL 保留",
			content:        "![](http://cdn.example.com/photo.jpg)",
			expectContains: "http://cdn.example.com/photo.jpg",
		},
		{
			name:           "已上传图片 key 保留",
			content:        "![](img_v2_uploaded)",
			expectContains: "img_v2_uploaded",
		},
		{
			name:           "file_ key 保留",
			content:        "![](file_v2_abc123)",
			expectContains: "file_v2_abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, count, err := processAndUploadLocalImages(tt.content, "/tmp/nonexistent")
			if err != nil {
				t.Fatalf("不应该返回错误: %v", err)
			}
			if count != 0 {
				t.Errorf("不应该有任何上传，count=%d", count)
			}
			if !strings.Contains(result, tt.expectContains) {
				t.Errorf("结果不包含期望字符串 %q，结果: %s", tt.expectContains, result)
			}
		})
	}
}

func TestProcessAndUploadLocalImages_RejectsMissingMarkdownImage(t *testing.T) {
	_, count, err := processAndUploadLocalImages("![](nonexistent.png)", "/tmp/nonexistent")
	if count != 0 {
		t.Fatalf("上传计数 = %d，期望 0", count)
	}
	if err == nil || !strings.Contains(err.Error(), "本地图片不可用") {
		t.Fatalf("期望本地图片缺失错误，实际: %v", err)
	}
}

func TestProcessAndUploadLocalImages_TextContent(t *testing.T) {
	// 测试纯文本内容不应该被处理
	content := "Hello World, this is plain text."
	result, count, err := processAndUploadLocalImages(content, "/tmp")
	if err != nil {
		t.Fatalf("不应该返回错误: %v", err)
	}
	if count != 0 {
		t.Errorf("纯文本不应该处理图片，count=%d", count)
	}
	if result != content {
		t.Errorf("纯文本应该原样返回")
	}
}

func TestProcessAndUploadLocalImages_MixedContent(t *testing.T) {
	// 混合内容只要有一张本地图片失败，就必须终止，不能带本地路径继续发送。
	content := "Remote: ![](https://example.com/remote.png) and local: ![](local.png)"
	_, count, err := processAndUploadLocalImages(content, "/tmp/nonexistent")
	if count != 0 {
		t.Errorf("失败前不应该记录上传，count=%d", count)
	}
	if err == nil || !strings.Contains(err.Error(), "本地图片不可用") {
		t.Fatalf("期望本地图片缺失错误，实际: %v", err)
	}
}

// -------------------------------------------------------------------
// Markdown 图片正则测试
// -------------------------------------------------------------------

func TestMarkdownImageRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantAlt  string
		wantPath string
	}{
		{"简单图片", "![](path.png)", 1, "", "path.png"},
		{"有 alt 文本", "![alt text](image.jpg)", 1, "alt text", "image.jpg"},
		{"带空格", "![  alt  ](path/gif)", 1, "  alt  ", "path/gif"},
		{"无匹配", "no image here", 0, "", ""},
		{"行内代码中的括号不匹配", "`![](a.png)`", 1, "", "a.png"}, // 正则会匹配到
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := markdownImageRegex.FindAllStringSubmatch(tt.input, -1)
			if len(matches) != tt.wantLen {
				t.Errorf("期望 %d 个匹配，实际 %d 个", tt.wantLen, len(matches))
				return
			}
			if tt.wantLen > 0 {
				if matches[0][1] != tt.wantAlt {
					t.Errorf("alt 文本期望 %q，实际 %q", tt.wantAlt, matches[0][1])
				}
				if matches[0][2] != tt.wantPath {
					t.Errorf("路径期望 %q，实际 %q", tt.wantPath, matches[0][2])
				}
			}
		})
	}
}

// -------------------------------------------------------------------
// resolveLocalPath 路径解析测试
// -------------------------------------------------------------------

func TestResolveLocalPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("获取用户主目录失败: %v", err)
	}
	tests := []struct {
		name     string
		path     string
		basePath string
		expected string
	}{
		{"相对路径", "test.png", "/tmp/dir", filepath.Join("/tmp/dir", "test.png")},
		{"绝对路径不变", "/abs/path/img.png", "/tmp/dir", "/abs/path/img.png"},
		{"用户主目录路径", "~/images/logo.png", "/tmp/dir", filepath.Join(homeDir, "images/logo.png")},
		{"子目录相对路径", "images/logo.png", "/tmp/dir", filepath.Join("/tmp/dir", "images/logo.png")},
		{"上级目录", "../assets/icon.png", "/tmp/dir", filepath.Join("/tmp/dir", "../assets/icon.png")},
		{"当前目录", "./screenshot.png", "/tmp/dir", filepath.Join("/tmp/dir", "./screenshot.png")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveLocalPath(tt.path, tt.basePath)
			if err != nil {
				t.Fatalf("resolveLocalPath(%q, %q) 返回错误: %v", tt.path, tt.basePath, err)
			}
			if result != tt.expected {
				t.Errorf("resolveLocalPath(%q, %q) = %q, 期望 %q", tt.path, tt.basePath, result, tt.expected)
			}
		})
	}
}
