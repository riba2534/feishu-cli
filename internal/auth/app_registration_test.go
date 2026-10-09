package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/config"
)

// regServers 为 feishu / lark 两个品牌各起一个 httptest 注册端点，并记录每个品牌收到的 action。
type regServers struct {
	mu      sync.Mutex
	actions map[config.Brand][]string
	urls    map[config.Brand]string
}

func newRegServers(t *testing.T, handlers map[config.Brand]func(action string, n int) string) *regServers {
	t.Helper()
	rs := &regServers{actions: map[config.Brand][]string{}, urls: map[config.Brand]string{}}
	for _, brand := range []config.Brand{config.BrandFeishu, config.BrandLark} {
		brand := brand
		h := handlers[brand]
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			action := form.Get("action")
			rs.mu.Lock()
			rs.actions[brand] = append(rs.actions[brand], action)
			n := len(rs.actions[brand])
			rs.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if h == nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"unexpected_brand"}`))
				return
			}
			_, _ = w.Write([]byte(h(action, n)))
		}))
		t.Cleanup(srv.Close)
		rs.urls[brand] = srv.URL
	}
	orig := appRegistrationEndpointFunc
	appRegistrationEndpointFunc = func(b config.Brand) string { return rs.urls[b] }
	origTick := regPollTick
	regPollTick = time.Millisecond
	t.Cleanup(func() {
		appRegistrationEndpointFunc = orig
		regPollTick = origTick
	})
	return rs
}

func (rs *regServers) got(b config.Brand) []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.actions[b]...)
}

// begin 始终发到飞书端；有效期读协议字段 expire_in（兼容 expires_in），缺省 600s；
// --brand lark 只影响确认页域名。
func TestRequestAppRegistration_ProtocolFields(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		brand      config.Brand
		wantExpire int
		wantHost   string
	}{
		{"expire_in 优先", `{"device_code":"dc","user_code":"UC","expire_in":900,"expires_in":300,"interval":3}`, config.BrandFeishu, 900, "https://open.feishu.cn/page/cli?user_code=UC"},
		{"兼容旧拼写 expires_in", `{"device_code":"dc","user_code":"UC","expires_in":420}`, config.BrandFeishu, 420, "https://open.feishu.cn/page/cli?user_code=UC"},
		{"缺失时默认 600s", `{"device_code":"dc","user_code":"UC"}`, config.BrandFeishu, 600, "https://open.feishu.cn/page/cli?user_code=UC"},
		{"lark 只换确认页域名", `{"device_code":"dc","user_code":"UC"}`, config.BrandLark, 600, "https://open.larksuite.com/page/cli?user_code=UC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := newRegServers(t, map[config.Brand]func(string, int) string{
				config.BrandFeishu: func(string, int) string { return tc.body },
			})
			resp, err := RequestAppRegistration(tc.brand)
			if err != nil {
				t.Fatal(err)
			}
			if resp.ExpiresIn != tc.wantExpire {
				t.Fatalf("ExpiresIn = %d, want %d", resp.ExpiresIn, tc.wantExpire)
			}
			if resp.Interval <= 0 {
				t.Fatalf("Interval 应有默认值: %d", resp.Interval)
			}
			if resp.VerificationURIComplete != tc.wantHost {
				t.Fatalf("确认页 = %q, want %q", resp.VerificationURIComplete, tc.wantHost)
			}
			if got := rs.got(config.BrandFeishu); len(got) != 1 || got[0] != "begin" {
				t.Fatalf("begin 应发到飞书端: feishu=%v lark=%v", got, rs.got(config.BrandLark))
			}
			if got := rs.got(config.BrandLark); len(got) != 0 {
				t.Fatalf("begin 不应发到 lark 端: %v", got)
			}
		})
	}
}

func TestRequestAppRegistration_MissingDeviceCode(t *testing.T) {
	newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(string, int) string { return `{"user_code":"UC"}` },
	})
	if _, err := RequestAppRegistration(config.BrandFeishu); err == nil || !strings.Contains(err.Error(), "device_code") {
		t.Fatalf("缺 device_code 应报错: %v", err)
	}
}

// Lark 租户：飞书端返回 tenant_brand=lark（可与 authorization_pending 同时到达）→ 立即切到 lark 端轮询，
// 生效品牌为 lark。
func TestPollAppRegistration_SwitchesToLarkTenant(t *testing.T) {
	rs := newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(string, int) string {
			return `{"error":"authorization_pending","user_info":{"tenant_brand":"lark"}}`
		},
		config.BrandLark: func(_ string, n int) string {
			if n == 1 {
				return `{"error":"authorization_pending"}`
			}
			return `{"client_id":"cli_lark","client_secret":"sec_lark","user_info":{"open_id":"ou_x","tenant_brand":"lark"}}`
		},
	})
	result, brand, err := PollAppRegistration(context.Background(), "dc", 1, 600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if brand != config.BrandLark || result.ClientID != "cli_lark" || result.ClientSecret != "sec_lark" || result.OpenID != "ou_x" {
		t.Fatalf("brand=%s result=%+v", brand, result)
	}
	if got := rs.got(config.BrandFeishu); len(got) != 1 {
		t.Fatalf("飞书端只应轮询一次即切换: %v", got)
	}
}

func TestPollAppRegistration_FeishuTenantAndIncompleteCredentials(t *testing.T) {
	newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(_ string, n int) string {
			switch n {
			case 1:
				return `{"error":"slow_down"}`
			case 2:
				// 无错误但缺 client_secret：继续轮询，而不是返回不完整凭证
				return `{"client_id":"cli_f","user_info":{"tenant_brand":"feishu"}}`
			default:
				return `{"client_id":"cli_f","client_secret":"sec_f","user_info":{"tenant_brand":"feishu"}}`
			}
		},
	})
	result, brand, err := PollAppRegistration(context.Background(), "dc", 1, 600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if brand != config.BrandFeishu || result.ClientSecret != "sec_f" {
		t.Fatalf("brand=%s result=%+v", brand, result)
	}
}

func TestPollAppRegistration_TerminalErrors(t *testing.T) {
	cases := map[string]error{
		`{"error":"access_denied"}`: ErrRegistrationDenied,
		`{"error":"expired_token"}`: ErrRegistrationExpired,
	}
	for body, want := range cases {
		newRegServers(t, map[config.Brand]func(string, int) string{
			config.BrandFeishu: func(string, int) string { return body },
		})
		_, _, err := PollAppRegistration(context.Background(), "dc", 1, 600, nil)
		if !errors.Is(err, want) {
			t.Fatalf("%s: err=%v want %v", body, err, want)
		}
	}

	// 无 tenant_brand：按飞书处理
	newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(string, int) string {
			return `{"client_id":"c","client_secret":"s"}`
		},
	})
	if _, brand, err := PollAppRegistration(context.Background(), "dc", 1, 600, nil); err != nil || brand != config.BrandFeishu {
		t.Fatalf("无 tenant_brand 时按飞书处理: brand=%s err=%v", brand, err)
	}

	// 已切到 lark 端后，最终凭证又声明 feishu：签发域与声明矛盾，视为协议错误
	newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(string, int) string {
			return `{"error":"authorization_pending","user_info":{"tenant_brand":"lark"}}`
		},
		config.BrandLark: func(string, int) string {
			return `{"client_id":"c","client_secret":"s","user_info":{"tenant_brand":"feishu"}}`
		},
	})
	if _, _, err := PollAppRegistration(context.Background(), "dc", 1, 600, nil); err == nil || !strings.Contains(err.Error(), "矛盾") {
		t.Fatalf("品牌矛盾应报错: %v", err)
	}

	// 一直 pending：按有效期超时
	newRegServers(t, map[config.Brand]func(string, int) string{
		config.BrandFeishu: func(string, int) string { return `{"error":"authorization_pending"}` },
	})
	if _, _, err := PollAppRegistration(context.Background(), "dc", 1, 30, nil); !errors.Is(err, ErrRegistrationTimedOut) {
		t.Fatalf("应超时: %v", err)
	}
}
