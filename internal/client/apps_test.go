package client

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseSparkResponse(t *testing.T) {
	t.Run("success returns data subobject", func(t *testing.T) {
		raw := []byte(`{"code":0,"msg":"ok","data":{"app":{"app_id":"app_xxx"}}}`)
		data, err := parseSparkResponse(http.StatusOK, raw)
		if err != nil {
			t.Fatal(err)
		}
		app, ok := data["app"].(map[string]any)
		if !ok || app["app_id"] != "app_xxx" {
			t.Fatalf("data = %#v", data)
		}
	})

	t.Run("biz code!=0 errors with msg", func(t *testing.T) {
		raw := []byte(`{"code":1061002,"msg":"permission denied","data":{}}`)
		_, err := parseSparkResponse(http.StatusOK, raw)
		if err == nil || !strings.Contains(err.Error(), "1061002") {
			t.Fatalf("want code error, got %v", err)
		}
	})

	t.Run("biz code!=0 surfaces data.error.hint when msg empty", func(t *testing.T) {
		raw := []byte(`{"code":1,"msg":"","data":{"error":{"hint":"missing name"}}}`)
		_, err := parseSparkResponse(http.StatusOK, raw)
		if err == nil || !strings.Contains(err.Error(), "missing name") {
			t.Fatalf("want hint surfaced, got %v", err)
		}
	})

	t.Run("HTTP 4xx errors with body preview", func(t *testing.T) {
		_, err := parseSparkResponse(http.StatusForbidden, []byte(`forbidden`))
		if err == nil || !strings.Contains(err.Error(), "403") {
			t.Fatalf("want http error, got %v", err)
		}
	})

	t.Run("no data subobject returns whole result", func(t *testing.T) {
		raw := []byte(`{"code":0,"msg":"ok","page_token":"t"}`)
		data, err := parseSparkResponse(http.StatusOK, raw)
		if err != nil {
			t.Fatal(err)
		}
		if data["page_token"] != "t" {
			t.Fatalf("data = %#v", data)
		}
	})
}

func TestParseHTMLPublishResponse(t *testing.T) {
	t.Run("success extracts only url (drops sibling fields)", func(t *testing.T) {
		raw := []byte(`{"code":0,"msg":"ok","data":{"url":"https://x.feishu.cn/app/app_x","status":1,"release_id":"r1"}}`)
		out, err := parseHTMLPublishResponse(http.StatusOK, raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out["url"] != "https://x.feishu.cn/app/app_x" {
			t.Fatalf("want exactly {url}, got %#v", out)
		}
	})

	t.Run("empty url yields empty map (no nil-key)", func(t *testing.T) {
		out, err := parseHTMLPublishResponse(http.StatusOK, []byte(`{"code":0,"msg":"ok","data":{}}`))
		if err != nil || len(out) != 0 {
			t.Fatalf("want empty map, got %#v err=%v", out, err)
		}
	})

	t.Run("build-failed code surfaces hint", func(t *testing.T) {
		_, err := parseHTMLPublishResponse(http.StatusOK, []byte(`{"code":90001,"msg":"build failed"}`))
		if err == nil || !strings.Contains(err.Error(), "90001") || !strings.Contains(err.Error(), "dry-run") {
			t.Fatalf("want 90001 + hint, got %v", err)
		}
	})

	t.Run("app-not-found code surfaces hint", func(t *testing.T) {
		_, err := parseHTMLPublishResponse(http.StatusOK, []byte(`{"code":90002,"msg":"not found"}`))
		if err == nil || !strings.Contains(err.Error(), "90002") {
			t.Fatalf("want 90002 error, got %v", err)
		}
	})
}

func TestSparkHTMLPublishHint(t *testing.T) {
	if sparkHTMLPublishHint(sparkErrCodeBuildFailed) == "" {
		t.Error("build-failed code should have a hint")
	}
	if sparkHTMLPublishHint(sparkErrCodeAppNotFound) == "" {
		t.Error("app-not-found code should have a hint")
	}
	if sparkHTMLPublishHint(12345) != "" {
		t.Error("unknown code should have no hint")
	}
}

func TestSparkBasePath(t *testing.T) {
	if SparkBasePath != "/open-apis/spark/v1" {
		t.Fatalf("SparkBasePath = %q", SparkBasePath)
	}
}

func TestParseHTMLPublishResponse_HTTPError(t *testing.T) {
	t.Run("4xx 透出状态码与 body", func(t *testing.T) {
		_, err := parseHTMLPublishResponse(http.StatusForbidden, []byte(`{"msg":"forbidden"}`))
		if err == nil || !strings.Contains(err.Error(), "403") {
			t.Fatalf("HTTP 403 应返回含 403 的 error，实际: %v", err)
		}
	})
	t.Run("空 body 的 5xx 仍报状态码", func(t *testing.T) {
		_, err := parseHTMLPublishResponse(http.StatusBadGateway, nil)
		if err == nil || !strings.Contains(err.Error(), "502") {
			t.Fatalf("空 body 的 502 应返回含 502 的 error，实际: %v", err)
		}
	})
	t.Run("成功仍只白名单提取 url", func(t *testing.T) {
		data, err := parseHTMLPublishResponse(http.StatusOK, []byte(`{"code":0,"data":{"url":"https://example.feishu.cn/x"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if data["url"] != "https://example.feishu.cn/x" {
			t.Fatalf("data = %#v", data)
		}
	})
}
