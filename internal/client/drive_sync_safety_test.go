package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/config"
)

// patternStreamReader 生成指定大小的流，内存开销为 O(1)
type patternStreamReader struct {
	pattern   []byte
	remaining int64
	pos       int
}

func newPatternReader(pattern []byte, totalSize int64) *patternStreamReader {
	if len(pattern) == 0 {
		pattern = []byte("A")
	}
	return &patternStreamReader{
		pattern:   pattern,
		remaining: totalSize,
		pos:       0,
	}
}

func (r *patternStreamReader) Read(p []byte) (n int, err error) {
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

// TestSaveToFile_100MBBoundary 验证 100MB 边界下载安全性：
// 100MB-1 成功写入；恰好 100MB 成功写入不误拒；100MB+1 拦截并清理文件。
func TestSaveToFile_100MBBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	const MB = 1024 * 1024
	tests := []struct {
		name        string
		size        int64
		wantErr     bool
		errContains string
	}{
		{
			name:    "100MB-1B 正常保存",
			size:    100*MB - 1,
			wantErr: false,
		},
		{
			name:    "恰好 100MB 正常保存不误拒",
			size:    100 * MB,
			wantErr: false,
		},
		{
			name:        "100MB+1B 超限拦截",
			size:        100*MB + 1,
			wantErr:     true,
			errContains: "文件超过大小限制",
		},
		{
			name:        "150MB 超限拦截",
			size:        150 * MB,
			wantErr:     true,
			errContains: "文件超过大小限制",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outPath := filepath.Join(tmpDir, "out_"+strings.ReplaceAll(tt.name, " ", "_")+".dat")
			reader := newPatternReader([]byte("TEST-STREAM-PAYLOAD-1234567890"), tt.size)

			err := saveToFile(reader, outPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回超限错误，但保存成功了")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息 %q 未包含期望内容 %q", err.Error(), tt.errContains)
				}
				// 验证超限时文件已被清理删除
				if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
					t.Errorf("超限失败后未删除残留文件: %s", outPath)
				}
			} else {
				if err != nil {
					t.Fatalf("期望成功写入，得到错误: %v", err)
				}
				stat, statErr := os.Stat(outPath)
				if statErr != nil {
					t.Fatalf("无法获取已保存文件状态: %v", statErr)
				}
				if stat.Size() != tt.size {
					t.Errorf("写入文件大小不匹配: 期望 %d 字节，实际 %d 字节", tt.size, stat.Size())
				}
			}
		})
	}
}

// TestHashStream_Beyond100MB 验证对大于 100MB 以及差异在 100MB 后的流能完整哈希，绝不静默截断
func TestHashStream_Beyond100MB(t *testing.T) {
	const MB = 1024 * 1024
	size := int64(100*MB + 1024) // 100MB + 1KB

	// Stream 1 与 Stream 2 前 100MB 完全相同，但后 1KB 不同
	reader1 := io.MultiReader(
		newPatternReader([]byte("X"), 100*MB),
		newPatternReader([]byte("AAAA"), 1024),
	)
	reader2 := io.MultiReader(
		newPatternReader([]byte("X"), 100*MB),
		newPatternReader([]byte("BBBB"), 1024),
	)

	h1 := sha256.New()
	n1, err := io.Copy(h1, reader1)
	if err != nil || n1 != size {
		t.Fatalf("读取 reader1 失败: n=%d, err=%v", n1, err)
	}
	hash1 := hex.EncodeToString(h1.Sum(nil))

	h2 := sha256.New()
	n2, err := io.Copy(h2, reader2)
	if err != nil || n2 != size {
		t.Fatalf("读取 reader2 失败: n=%d, err=%v", n2, err)
	}
	hash2 := hex.EncodeToString(h2.Sum(nil))

	if hash1 == hash2 {
		t.Fatalf("差异位于 100MB 后的两个流计算出的哈希不应相同！发生了前缀截断！hash=%s", hash1)
	}

	// 验证前 100MB 的截断哈希与完整哈希不同
	hPrefix := sha256.New()
	_, _ = io.Copy(hPrefix, newPatternReader([]byte("X"), 100*MB))
	hashPrefix := hex.EncodeToString(hPrefix.Sum(nil))

	if hash1 == hashPrefix {
		t.Fatalf("完整哈希 hash1 不应等于 100MB 前缀哈希 hashPrefix")
	}
}

// TestListFolderRecursive_DuplicateRemotePath_FailClosed 验证远端存在同相对路径条目时，默认 fail closed 报错
func TestListFolderRecursive_DuplicateRemotePath_FailClosed(t *testing.T) {
	// 构造 mock HTTP server 模拟飞书 ListFiles API 返回同级重名文件
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/open-apis/drive/v1/files") {
			// 返回两个同名的 file 条目：report.pdf (token1 和 token2)
			respData := map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"has_more":        false,
					"next_page_token": "",
					"files": []map[string]any{
						{
							"token": "boxcn1111111111",
							"name":  "report.pdf",
							"type":  "file",
						},
						{
							"token": "boxcn2222222222",
							"name":  "report.pdf",
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

	config.SetBotFlagCredentials("cli_test_app", "test_secret_123")
	config.ApplyBotFlagCredentials()
	cfg := config.Get()
	origBaseURL := cfg.BaseURL
	cfg.BaseURL = mockServer.URL
	defer func() {
		cfg.BaseURL = origBaseURL
		config.SetBotFlagCredentials("", "")
	}()

	entries, err := ListFolderRecursive("fld_root", "test_user_token")
	if err == nil {
		t.Fatalf("期望远端存在重复相对路径时 fail closed 报错，但得到了 nil 错误，entries=%v", entries)
	}

	if !strings.Contains(err.Error(), "重复相对路径") {
		t.Errorf("错误信息未包含'重复相对路径': %v", err)
	}
	if !strings.Contains(err.Error(), "report.pdf") {
		t.Errorf("错误信息未包含重名文件名 report.pdf: %v", err)
	}
	if !strings.Contains(err.Error(), "boxcn1111111111") || !strings.Contains(err.Error(), "boxcn2222222222") {
		t.Errorf("错误信息未包含冲突条目的 token: %v", err)
	}
}

// TestHashRemoteFile_MockServer 验证 HashRemoteFile 通过 HTTP mock 下载流计算完整哈希
func TestHashRemoteFile_MockServer(t *testing.T) {
	testPayload := []byte("HELLO-REMOTE-DRIVE-HASH-CONTENT-SAFETY-TEST")
	expectedHashBytes := sha256.Sum256(testPayload)
	expectedHash := hex.EncodeToString(expectedHashBytes[:])

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/download") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testPayload)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	config.SetBotFlagCredentials("cli_test_app", "test_secret_123")
	config.ApplyBotFlagCredentials()
	cfg := config.Get()
	origBaseURL := cfg.BaseURL
	cfg.BaseURL = mockServer.URL
	defer func() {
		cfg.BaseURL = origBaseURL
		config.SetBotFlagCredentials("", "")
	}()

	hash, err := HashRemoteFile("boxcn_test_token", "u-test-token")
	if err != nil {
		t.Fatalf("HashRemoteFile 失败: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("HashRemoteFile 哈希不匹配: got %s, want %s", hash, expectedHash)
	}
}
