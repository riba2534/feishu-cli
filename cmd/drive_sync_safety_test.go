package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
)

// patternStreamReader 用于在测试中生成大流，内存开销为 O(1)
type testPatternReader struct {
	pattern   []byte
	remaining int64
	pos       int
}

func newTestPatternReader(pattern []byte, totalSize int64) *testPatternReader {
	if len(pattern) == 0 {
		pattern = []byte("A")
	}
	return &testPatternReader{
		pattern:   pattern,
		remaining: totalSize,
		pos:       0,
	}
}

func (r *testPatternReader) Read(p []byte) (n int, err error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	toRead := int64(len(p))
	if toRead > r.remaining {
		toRead = r.remaining
	}
	for i := int64(0); i < toRead; i++ {
		p[i] = r.pattern[r.pos%len(r.pattern)]
		r.pos++
	}
	r.remaining -= toRead
	return int(toRead), nil
}

// setupCmdTestConfig 配置 mock server 与凭证
func setupCmdTestConfig(t *testing.T, mockURL string) func() {
	tempHome := t.TempDir()
	restoreHome := profile.SetHomeFunc(func() (string, error) {
		return tempHome, nil
	})
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	config.SetBotFlagCredentials("cli_test_app", "test_secret_123")
	config.ApplyBotFlagCredentials()
	cfg := config.Get()
	origBaseURL := cfg.BaseURL
	origUserAccessToken := cfg.UserAccessToken
	cfg.UserAccessToken = ""
	cfg.BaseURL = mockURL
	return func() {
		restoreHome()
		cfg.BaseURL = origBaseURL
		cfg.UserAccessToken = origUserAccessToken
		config.SetBotFlagCredentials("", "")
	}
}

func mockAuthHandler(w http.ResponseWriter, r *http.Request) bool {
	if strings.Contains(r.URL.Path, "/tenant_access_token") || strings.Contains(r.URL.Path, "/refresh_access_token") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":                0,
			"msg":                 "ok",
			"tenant_access_token": "t-mock-token",
			"access_token":        "u-mock-token",
			"expire":              7200,
		})
		return true
	}
	return false
}

// TestDriveStatus_DuplicateRemotePath_FailClosed 验证 drive status 遇到远端重复相对路径时 fail closed 报错中止
func TestDriveStatus_DuplicateRemotePath_FailClosed(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{
							"token": "boxcn_dup_1",
							"name":  "conflict.txt",
							"type":  "file",
						},
						{
							"token": "boxcn_dup_2",
							"name":  "conflict.txt",
							"type":  "file",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(respData)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	cleanup := setupCmdTestConfig(t, mockServer.URL)
	defer cleanup()

	tmpDir := t.TempDir()
	// 在 cwd 子树内进行测试
	cwd, _ := os.Getwd()
	relDir, _ := filepath.Rel(cwd, tmpDir)
	if strings.HasPrefix(relDir, "..") {
		// tmpDir 不在 cwd 下，在当前目录建临时子目录
		subDir := filepath.Join(cwd, "test_status_dup_tmp")
		_ = os.MkdirAll(subDir, 0755)
		defer os.RemoveAll(subDir)
		tmpDir = subDir
	}

	cmd := driveStatusCmd
	cmd.Flags().Set("folder-token", "fld_test_root")
	cmd.Flags().Set("local-dir", tmpDir)
	cmd.Flags().Set("output", "json")
	cmd.Flags().Set("user-access-token", "u-test-token")

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatalf("期望远端重复相对路径时报错中止，但执行成功")
	}
	if !strings.Contains(err.Error(), "重复相对路径") {
		t.Errorf("错误信息未包含'重复相对路径': %v", err)
	}
}

// TestDrivePull_DuplicateRemotePath_FailClosed 验证 drive pull 遇到远端重复相对路径时 fail closed 报错中止
func TestDrivePull_DuplicateRemotePath_FailClosed(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{
							"token": "boxcn_dup_1",
							"name":  "data.csv",
							"type":  "file",
						},
						{
							"token": "boxcn_dup_2",
							"name":  "data.csv",
							"type":  "file",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(respData)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	cleanup := setupCmdTestConfig(t, mockServer.URL)
	defer cleanup()

	cwd, _ := os.Getwd()
	tmpDir := filepath.Join(cwd, "test_pull_dup_tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	cmd := drivePullCmd
	cmd.Flags().Set("folder-token", "fld_test_root")
	cmd.Flags().Set("local-dir", tmpDir)
	cmd.Flags().Set("user-access-token", "u-test-token")

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatalf("期望 pull 在远端重复路径时 fail closed 报错中止，但执行成功")
	}
	if !strings.Contains(err.Error(), "重复相对路径") {
		t.Errorf("错误信息未包含'重复相对路径': %v", err)
	}
}

// TestDrivePush_DuplicateRemotePath_FailClosed 验证 drive push 遇到远端重复相对路径时 fail closed 报错中止
func TestDrivePush_DuplicateRemotePath_FailClosed(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{
							"token": "boxcn_dup_1",
							"name":  "pkg.tar",
							"type":  "file",
						},
						{
							"token": "boxcn_dup_2",
							"name":  "pkg.tar",
							"type":  "file",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(respData)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	cleanup := setupCmdTestConfig(t, mockServer.URL)
	defer cleanup()

	cwd, _ := os.Getwd()
	tmpDir := filepath.Join(cwd, "test_push_dup_tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "local_file.txt"), []byte("content"), 0644)

	cmd := drivePushCmd
	cmd.Flags().Set("folder-token", "fld_test_root")
	cmd.Flags().Set("local-dir", tmpDir)
	cmd.Flags().Set("user-access-token", "u-test-token")

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatalf("期望 push 在远端重复路径时 fail closed 报错中止，但执行成功")
	}
	if !strings.Contains(err.Error(), "重复相对路径") {
		t.Errorf("错误信息未包含'重复相对路径': %v", err)
	}
}

// TestDriveStatus_DifferenceBeyond100MB 验证差异位于 100MB 之后的文件在 status 比对时被正确归入 modified 桶
func TestDriveStatus_DifferenceBeyond100MB(t *testing.T) {
	const MB = 1024 * 1024

	// 本地文件：前 100MB 为 'X'，后 1KB 为 'AAAA'
	cwd, _ := os.Getwd()
	tmpDir := filepath.Join(cwd, "test_status_diff_tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	localFilePath := filepath.Join(tmpDir, "big_file.bin")
	localFile, err := os.Create(localFilePath)
	if err != nil {
		t.Fatalf("创建本地大文件失败: %v", err)
	}
	_, _ = io.Copy(localFile, newTestPatternReader([]byte("X"), 100*MB))
	_, _ = io.Copy(localFile, newTestPatternReader([]byte("AAAA"), 1024))
	localFile.Close()

	// 远端文件：前 100MB 为 'X'，后 1KB 为 'BBBB'
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files/boxcn_big/download") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, newTestPatternReader([]byte("X"), 100*MB))
			_, _ = io.Copy(w, newTestPatternReader([]byte("BBBB"), 1024))
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{
							"token": "boxcn_big",
							"name":  "big_file.bin",
							"type":  "file",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(respData)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	cleanup := setupCmdTestConfig(t, mockServer.URL)
	defer cleanup()

	// 捕获 stdout
	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	cmd := driveStatusCmd
	cmd.Flags().Set("folder-token", "fld_test_root")
	cmd.Flags().Set("local-dir", tmpDir)
	cmd.Flags().Set("output", "json")
	cmd.Flags().Set("user-access-token", "u-test-token")

	runErr := cmd.RunE(cmd, []string{})
	wOut.Close()
	os.Stdout = oldStdout

	if runErr != nil {
		t.Fatalf("drive status 运行失败: %v", runErr)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rOut)

	var statusResult struct {
		Modified []struct {
			RelPath string `json:"rel_path"`
		} `json:"modified"`
		Unchanged []struct {
			RelPath string `json:"rel_path"`
		} `json:"unchanged"`
	}
	if err := json.Unmarshal(buf.Bytes(), &statusResult); err != nil {
		t.Fatalf("解析 status JSON 输出失败: %v, raw output: %s", err, buf.String())
	}

	if len(statusResult.Modified) != 1 || statusResult.Modified[0].RelPath != "big_file.bin" {
		t.Errorf("期望 big_file.bin 归入 modified，实际 modified=%v, unchanged=%v",
			statusResult.Modified, statusResult.Unchanged)
	}
	if len(statusResult.Unchanged) != 0 {
		t.Errorf("unchanged 应为空，实际为 %v", statusResult.Unchanged)
	}
}

// TestDrivePull_BotStreamingBeyond100MB_Safety 验证 Bot 身份 pull 走流式下载：
// 旧实现经 SDK 把整个文件读进内存并对 Bot 路径设 100MB 上限（100MB+1 直接失败）；
// 现在恰好 100MB 与 100MB+1 都应完整落盘。同时验证任一文件下载失败（403 业务错误）时
// --delete-local 被跳过、本地孤儿文件保留，且失败文件不残留半截内容。
func TestDrivePull_BotStreamingBeyond100MB_Safety(t *testing.T) {
	const MB = 1024 * 1024

	var tenantAuth sync.Map
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		if strings.Contains(r.URL.Path, "/download") {
			tenantAuth.Store(r.Header.Get("Authorization"), true)
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files/boxcn_100m_exact/download") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, newTestPatternReader([]byte("X"), 100*MB))
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files/boxcn_100m_plus/download") {
			w.WriteHeader(http.StatusOK)
			_, _ = io.Copy(w, newTestPatternReader([]byte("Y"), 100*MB+1))
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files/boxcn_forbidden/download") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"code":1061004,"msg":"forbidden"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{"token": "boxcn_100m_exact", "name": "exact_100m.bin", "type": "file"},
						{"token": "boxcn_100m_plus", "name": "plus_100m.bin", "type": "file"},
						{"token": "boxcn_forbidden", "name": "forbidden.bin", "type": "file"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(respData)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	cleanup := setupCmdTestConfig(t, mockServer.URL)
	defer cleanup()

	cwd, _ := os.Getwd()
	tmpDir := filepath.Join(cwd, "test_pull_boundary_tmp")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	// 本地孤儿文件：pull 有任何下载失败时 --delete-local 不应删除它
	orphanPath := filepath.Join(tmpDir, "orphan.txt")
	_ = os.WriteFile(orphanPath, []byte("preserve me"), 0644)
	// 失败文件的本地旧版本：下载失败时必须保持原样（原子写，不删用户原文件）
	forbiddenPath := filepath.Join(tmpDir, "forbidden.bin")
	_ = os.WriteFile(forbiddenPath, []byte("old local copy"), 0644)

	cmd := drivePullCmd
	cmd.Flags().Set("folder-token", "fld_test_root")
	cmd.Flags().Set("local-dir", tmpDir)
	cmd.Flags().Set("user-access-token", "")
	cmd.Flags().Set("delete-local", "true")
	cmd.Flags().Set("yes", "true")
	defer func() {
		cmd.Flags().Set("delete-local", "false")
		cmd.Flags().Set("yes", "false")
	}()

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatalf("存在 403 下载失败，pull 应当返回失败错误")
	}

	for name, want := range map[string]int64{"exact_100m.bin": 100 * MB, "plus_100m.bin": 100*MB + 1} {
		stat, statErr := os.Stat(filepath.Join(tmpDir, name))
		if statErr != nil {
			t.Fatalf("%s 应流式下载成功（Bot 路径不再有 100MB 上限）: %v", name, statErr)
		}
		if stat.Size() != want {
			t.Errorf("%s 大小不匹配: got %d, want %d", name, stat.Size(), want)
		}
	}

	if got, _ := os.ReadFile(forbiddenPath); string(got) != "old local copy" {
		t.Errorf("下载失败时本地同名文件必须保持原样，got %q", got)
	}
	if _, err := os.Stat(orphanPath); err != nil {
		t.Errorf("由于存在下载失败，delete-local 应当跳过，orphan.txt 应当保留: %v", err)
	}
	// Bot 下载必须携带 tenant token 走流式 Bearer 请求
	sawTenant := false
	tenantAuth.Range(func(k, _ any) bool {
		if k.(string) == "Bearer t-mock-token" {
			sawTenant = true
		}
		return true
	})
	if !sawTenant {
		t.Errorf("Bot 下载请求应携带 Bearer tenant token")
	}
}

// TestHashCalculation 校验本地与远端 SHA-256 计算一致性
func TestHashCalculation(t *testing.T) {
	payload := []byte("VALIDATE-SHA256-HASH-CONSISTENCY-PAYLOAD")
	expected := hex.EncodeToString(func() []byte {
		h := sha256.Sum256(payload)
		return h[:]
	}())

	tmpFile := filepath.Join(t.TempDir(), "f.txt")
	_ = os.WriteFile(tmpFile, payload, 0644)

	localHash, err := clientHashLocal(tmpFile)
	if err != nil {
		t.Fatalf("计算本地哈希失败: %v", err)
	}
	if localHash != expected {
		t.Errorf("本地哈希不匹配: got %s, want %s", localHash, expected)
	}
}

func clientHashLocal(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
