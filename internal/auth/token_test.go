package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUserToken_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		token UserToken
		want  bool
	}{
		{
			name: "有效令牌（未过期且剩余 > 5 分钟）",
			token: UserToken{
				AccessToken: "u-test",
				ExpiresAt:   time.Now().Unix() + 600, // 10 分钟后过期
			},
			want: true,
		},
		{
			name: "即将过期（剩余 < 5 分钟）",
			token: UserToken{
				AccessToken: "u-test",
				ExpiresAt:   time.Now().Unix() + 200, // 3 分钟后过期
			},
			want: false,
		},
		{
			name: "已过期",
			token: UserToken{
				AccessToken: "u-test",
				ExpiresAt:   time.Now().Unix() - 100,
			},
			want: false,
		},
		{
			name: "空令牌",
			token: UserToken{
				AccessToken: "",
				ExpiresAt:   time.Now().Unix() + 600,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, 期望 %v", got, tt.want)
			}
		})
	}
}

func TestUserToken_IsRefreshable(t *testing.T) {
	tests := []struct {
		name  string
		token UserToken
		want  bool
	}{
		{
			name: "可刷新",
			token: UserToken{
				RefreshToken:     "ur-test",
				RefreshExpiresAt: time.Now().Unix() + 86400,
			},
			want: true,
		},
		{
			name: "refresh_token 已过期",
			token: UserToken{
				RefreshToken:     "ur-test",
				RefreshExpiresAt: time.Now().Unix() - 100,
			},
			want: false,
		},
		{
			name: "无 refresh_token",
			token: UserToken{
				RefreshToken:     "",
				RefreshExpiresAt: time.Now().Unix() + 86400,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsRefreshable(); got != tt.want {
				t.Errorf("IsRefreshable() = %v, 期望 %v", got, tt.want)
			}
		})
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	// 使用临时目录模拟 home
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// 创建配置目录
	configDir := filepath.Join(tmpDir, ".feishu-cli")
	os.MkdirAll(configDir, 0700)

	token := &UserToken{
		AccessToken:      "u-test-token",
		RefreshToken:     "ur-test-refresh",
		ExpiresAt:        time.Now().Unix() + 7200,
		RefreshExpiresAt: time.Now().Unix() + 2592000,
		TokenType:        "Bearer",
		Scope:            "docx:document wiki:wiki:readonly",
	}

	// 保存
	if err := SaveToken(token); err != nil {
		t.Fatalf("SaveToken() 失败: %v", err)
	}

	// 验证文件权限
	tokenPath := filepath.Join(configDir, "user_token.json")
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("token 文件不存在: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("文件权限 = %o, 期望 0600", info.Mode().Perm())
	}

	// 加载
	loaded, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() 失败: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadToken() 返回 nil")
	}
	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken = %q, 期望 %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken = %q, 期望 %q", loaded.RefreshToken, token.RefreshToken)
	}
	if loaded.Scope != token.Scope {
		t.Errorf("Scope = %q, 期望 %q", loaded.Scope, token.Scope)
	}
}

func TestLoadToken_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() 返回错误: %v", err)
	}
	if token != nil {
		t.Error("期望返回 nil，但返回了 token")
	}
}

func TestDeleteToken(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// 先保存
	configDir := filepath.Join(tmpDir, ".feishu-cli")
	os.MkdirAll(configDir, 0700)

	token := &UserToken{
		AccessToken: "u-test",
		ExpiresAt:   time.Now().Unix() + 7200,
	}
	if err := SaveToken(token); err != nil {
		t.Fatalf("SaveToken() 失败: %v", err)
	}

	// 删除
	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken() 失败: %v", err)
	}

	// 再次加载应返回 nil
	loaded, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() 返回错误: %v", err)
	}
	if loaded != nil {
		t.Error("删除后仍能加载到 token")
	}

	// 重复删除不应报错
	if err := DeleteToken(); err != nil {
		t.Errorf("重复删除返回错误: %v", err)
	}
}

func TestParseTokenMode(t *testing.T) {
	tests := []struct {
		input string
		want  TokenMode
		err   bool
	}{
		{"", TokenModeAuto, false},
		{"auto", TokenModeAuto, false},
		{"user", TokenModeUser, false},
		{"tenant", TokenModeTenant, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTokenMode(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("ParseTokenMode(%q) error = %v, 期望 err = %v", tt.input, err, tt.err)
			}
			if got != tt.want {
				t.Errorf("ParseTokenMode(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}
