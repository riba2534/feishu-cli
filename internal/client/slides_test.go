package client

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildPresentationXML_NamespaceHTTPS 验证 SML namespace 必须使用 https。
func TestBuildPresentationXML_NamespaceHTTPS(t *testing.T) {
	xml := buildPresentationXML("测试标题 & <Deck>", 1920, 1080)

	wantNamespace := `xmlns="https://www.larkoffice.com/sml/2.0"`
	if !strings.Contains(xml, wantNamespace) {
		t.Errorf("XML 必须包含 HTTPS namespace %q, 实际得到:\n%s", wantNamespace, xml)
	}

	// 严禁 HTTP
	if strings.Contains(xml, `xmlns="http://www.larkoffice.com/sml/2.0"`) {
		t.Errorf("XML 不得使用 HTTP namespace: %s", xml)
	}

	if !strings.Contains(xml, `<title>测试标题 &amp; &lt;Deck&gt;</title>`) {
		t.Errorf("XML 转义不正确: %s", xml)
	}
	if !strings.Contains(xml, `width="1920"`) || !strings.Contains(xml, `height="1080"`) {
		t.Errorf("XML 宽高属性不正确: %s", xml)
	}
}

// TestIsOfficePresentation_Contract 覆盖 isOfficePresentation（委托 IsLocalOfficeToken）的正例与负例，
// 并验证 slidesMediaParentType 与判定结果严格对齐。
func TestIsOfficePresentation_Contract(t *testing.T) {
	// 交织 marker：0-based 下标 [4],[9],[14],[19],[24] 分别为 'O', 'F', 'L', '0', 'X'
	officialPositiveFixture := "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"

	cases := []struct {
		name  string
		token string
		want  bool
	}{
		// --- 正例 ---
		{"legacy fake_office_ 前缀", "fake_office_ppt_123456", true},
		{"legacy fake_office_ 前缀自身", "fake_office_", true},
		{"legacy local_office_ 前缀", "local_office_deck_888", true},
		{"legacy local_office_ 前缀自身", "local_office_", true},
		{"28 字符交织 OFL0X marker fixture", officialPositiveFixture, true},
		{"真实格式 28 字符交织 marker", "abcdOefghFijklLmnop0qrstXuvw", true},
		{"27 字符交织 marker（当前导入格式 OFL0X+21 随机+1 类型枚举）", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", true},
		{"29 字符交织 marker（长度只设下限）", "aaaaOaaaaFaaaaLaaaa0aaaaXaaaa", true},
		{"25 字符交织 marker（恰好容纳标记）", "aaaaOaaaaFaaaaLaaaa0aaaaX", true},

		// --- 负例 ---
		{"5-shifted marker fixture (下标 5,10,15,20,25 错位)", "aaaaaObbbbFccccLdddd0eeeeXff", false},
		{"普通原生 slides 28 字符 token (无 marker)", "zTqAwsEb4clrjOLd3drAcNZabcef", false},
		{"24 字符：差一位容纳不下 marker", "aaaaOaaaaFaaaaLaaaa0aaaa", false},
		{"错位 marker: 提前 1 位 (位置 3,8,13,18,23)", "aaaOaaaaFaaaaLaaaa0aaaaXaaaa", false},
		{"错位 marker: 滞后 1 位 (位置 5,10,15,20,25)", "aaaaaOaaaaFaaaaLaaaa0aaaaXaa", false},
		{"小写 marker 不匹配 (ofl0x)", "aaaaoaaaafaaaalaaaa0aaaaxaaa", false},
		{"部分字符不匹配 (第 24 位不是 X 而是 Y)", "aaaaOaaaaFaaaaLaaaa0aaaaYaaa", false},
		{"同长度原生 pptcn 交织 token", "abcdpefghpijkltmnopcqrstnuv", false},
		{"前缀出现在中间不匹配", "prefix_fake_office_123", false},
		{"普通短 token", "sldcnABC123", false},
		{"空 token", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isOfficePresentation(tc.token)
			if got != tc.want {
				t.Errorf("isOfficePresentation(%q) = %v, want %v", tc.token, got, tc.want)
			}
			// 验证 slidesMediaParentType 与 isOfficePresentation 结果严格对齐
			parentType := slidesMediaParentType(tc.token)
			if tc.want && parentType != "office_slide_file" {
				t.Errorf("slidesMediaParentType(%q) = %q, want office_slide_file", tc.token, parentType)
			}
			if !tc.want && parentType != "slide_file" {
				t.Errorf("slidesMediaParentType(%q) = %q, want slide_file", tc.token, parentType)
			}
		})
	}
}

// TestUploadSlidesMedia_ParentTypeSelection 验证 UploadSlidesMedia 在 wire 层发送正确的 parent_type。
func TestUploadSlidesMedia_ParentTypeSelection(t *testing.T) {
	var gotParentType string
	var gotParentNode string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				formName := p.FormName()
				data, _ := io.ReadAll(p)
				if formName == "parent_type" {
					gotParentType = string(data)
				}
				if formName == "parent_node" {
					gotParentNode = string(data)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"file_token":"boxcn_media_token_123"}}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	// 创建临时图片文件
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake png content"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. 原生 Slides deck 上传 -> slide_file
	gotParentType = ""
	gotParentNode = ""
	token, err := UploadSlidesMedia(imgPath, "test.png", "zTqAwsEb4clrjOLd3drAcNZabcef", "u-test")
	if err != nil {
		t.Fatalf("UploadSlidesMedia 失败: %v", err)
	}
	if token != "boxcn_media_token_123" {
		t.Errorf("返回 token 不符: %s", token)
	}
	if gotParentType != "slide_file" {
		t.Errorf("原生 deck parent_type 应为 slide_file，实际为 %s", gotParentType)
	}
	if gotParentNode != "zTqAwsEb4clrjOLd3drAcNZabcef" {
		t.Errorf("parent_node 不符: %s", gotParentNode)
	}

	// 2. 导入型 Office deck (legacy fake_office_) 上传 -> office_slide_file
	gotParentType = ""
	gotParentNode = ""
	token, err = UploadSlidesMedia(imgPath, "test.png", "fake_office_deck_999", "u-test")
	if err != nil {
		t.Fatalf("UploadSlidesMedia 失败: %v", err)
	}
	if gotParentType != "office_slide_file" {
		t.Errorf("Office deck parent_type 应为 office_slide_file，实际为 %s", gotParentType)
	}
	if gotParentNode != "fake_office_deck_999" {
		t.Errorf("parent_node 不符: %s", gotParentNode)
	}

	// 3. 导入型 Office deck (官方 28 字符 OFL0X marker) 上传 -> office_slide_file
	markerToken := "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"
	gotParentType = ""
	gotParentNode = ""
	token, err = UploadSlidesMedia(imgPath, "test.png", markerToken, "u-test")
	if err != nil {
		t.Fatalf("UploadSlidesMedia 失败: %v", err)
	}
	if gotParentType != "office_slide_file" {
		t.Errorf("28 字符 marker Office deck parent_type 应为 office_slide_file，实际为 %s", gotParentType)
	}
	if gotParentNode != markerToken {
		t.Errorf("parent_node 不符: %s", gotParentNode)
	}
}

// TestGetSlides 验证 GetSlides 读取演示文稿 XML、path escape 与 revision_id 默认值。
func TestGetSlides(t *testing.T) {
	var gotEscapedPath string
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscapedPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"xml_presentation": {
					"content": "<presentation xmlns=\"https://www.larkoffice.com/sml/2.0\"><title>My Presentation</title></presentation>",
					"presentation_id": "pres_123",
					"revision_id": 5
				}
			}
		}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	// 1. 默认 revision_id (0) -> 传递 -1 (最新版本)
	res, err := GetSlides("pres_123", 0, "u-test")
	if err != nil {
		t.Fatalf("GetSlides 意外报错: %v", err)
	}
	if gotEscapedPath != "/open-apis/slides_ai/v1/xml_presentations/pres_123" {
		t.Errorf("请求路径不符: %s", gotEscapedPath)
	}
	if gotQuery != "revision_id=-1" {
		t.Errorf("默认 query 应为 revision_id=-1，实际得到: %s", gotQuery)
	}
	if res.XmlPresentationID != "pres_123" {
		t.Errorf("presentation_id 不符: %s", res.XmlPresentationID)
	}
	if res.RevisionID != 5 {
		t.Errorf("revision_id 不符: %d", res.RevisionID)
	}
	if !strings.Contains(res.Content, "My Presentation") {
		t.Errorf("content 不符: %s", res.Content)
	}

	// 2. 指定 revision_id (10)
	res, err = GetSlides("pres_123", 10, "u-test")
	if err != nil {
		t.Fatalf("GetSlides 指定版本意外报错: %v", err)
	}
	if gotQuery != "revision_id=10" {
		t.Errorf("query 应为 revision_id=10，实际得到: %s", gotQuery)
	}

	// 3. Path 转义测试（含特殊字符）
	_, err = GetSlides("pres/special token", 0, "u-test")
	if err != nil {
		t.Fatalf("GetSlides path 转义意外报错: %v", err)
	}
	if gotEscapedPath != "/open-apis/slides_ai/v1/xml_presentations/pres%2Fspecial%20token" {
		t.Errorf("Path 应被转义，实际得到: %s", gotEscapedPath)
	}
}

// TestGetSlides_EmptyContentFailClosed 验证 xml_presentation.content 为空时 fail closed 报错。
func TestGetSlides_EmptyContentFailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"xml_presentation": {
					"content": "   ",
					"presentation_id": "pres_123",
					"revision_id": 1
				}
			}
		}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	_, err := GetSlides("pres_123", 0, "u-test")
	if err == nil {
		t.Fatal("content 为空时 GetSlides 应报错")
	}
	if !strings.Contains(err.Error(), "内容为空") {
		t.Errorf("错误信息不符: %v", err)
	}
}

// TestCreateSlides 验证 CreateSlides 创建演示文稿。
func TestCreateSlides(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"code": 0,
			"msg": "ok",
			"data": {
				"xml_presentation_id": "new_pres_999",
				"revision_id": 1
			}
		}`)
	}))
	t.Cleanup(srv.Close)
	setupTestConfig(t, srv.URL)

	res, err := CreateSlides(CreateSlidesOptions{
		Title:           "Quarterly Report",
		Width:           1920,
		Height:          1080,
		UserAccessToken: "u-test",
	})
	if err != nil {
		t.Fatalf("CreateSlides 失败: %v", err)
	}
	if res.XmlPresentationID != "new_pres_999" {
		t.Errorf("xml_presentation_id 不符: %s", res.XmlPresentationID)
	}

	var reqBody struct {
		XmlPresentation struct {
			Content string `json:"content"`
		} `json:"xml_presentation"`
	}
	if err := json.Unmarshal([]byte(gotBody), &reqBody); err != nil {
		t.Fatalf("解析请求体失败: %v", err)
	}
	if !strings.Contains(reqBody.XmlPresentation.Content, `xmlns="https://www.larkoffice.com/sml/2.0"`) {
		t.Errorf("创建请求体 XML 应包含 HTTPS namespace: %s", reqBody.XmlPresentation.Content)
	}
}
