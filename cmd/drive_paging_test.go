package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakePages 模拟服务端分页：pages[token] = (items, next, hasMore)。
type fakePage struct {
	items   []int
	next    string
	hasMore bool
}

func fakeFetcher(pages map[string]fakePage, calls *[]string) func(string) ([]int, string, bool, error) {
	return func(token string) ([]int, string, bool, error) {
		*calls = append(*calls, token)
		p, ok := pages[token]
		if !ok {
			return nil, "", false, fmt.Errorf("unexpected token %q", token)
		}
		return p.items, p.next, p.hasMore, nil
	}
}

func TestCollectListPages(t *testing.T) {
	pages := map[string]fakePage{
		"":   {items: []int{1, 2}, next: "p2", hasMore: true},
		"p2": {items: []int{3}, next: "p3", hasMore: true},
		"p3": {items: []int{4}, next: "", hasMore: false},
	}

	t.Run("single page keeps has_more and token", func(t *testing.T) {
		var calls []string
		res, err := collectListPages(listPageOptions{}, fakeFetcher(pages, &calls))
		if err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || !res.HasMore || res.NextPageToken != "p2" || len(res.Items) != 2 {
			t.Fatalf("got calls=%v res=%+v", calls, res)
		}
	})

	t.Run("page token resumes", func(t *testing.T) {
		var calls []string
		res, err := collectListPages(listPageOptions{PageToken: "p2"}, fakeFetcher(pages, &calls))
		if err != nil {
			t.Fatal(err)
		}
		if calls[0] != "p2" || res.NextPageToken != "p3" || !res.HasMore {
			t.Fatalf("got calls=%v res=%+v", calls, res)
		}
	})

	t.Run("page all collects everything", func(t *testing.T) {
		var calls []string
		res, err := collectListPages(listPageOptions{PageAll: true}, fakeFetcher(pages, &calls))
		if err != nil {
			t.Fatal(err)
		}
		if res.HasMore || res.NextPageToken != "" || len(res.Items) != 4 || res.Pages != 3 || res.Truncated {
			t.Fatalf("got calls=%v res=%+v", calls, res)
		}
	})

	t.Run("page limit truncates with token", func(t *testing.T) {
		var calls []string
		res, err := collectListPages(listPageOptions{PageAll: true, PageLimit: 2}, fakeFetcher(pages, &calls))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Truncated || !res.HasMore || res.NextPageToken != "p3" || len(res.Items) != 3 {
			t.Fatalf("got calls=%v res=%+v", calls, res)
		}
	})

	t.Run("repeated cursor stops with error", func(t *testing.T) {
		loop := map[string]fakePage{
			"":  {items: []int{1}, next: "a", hasMore: true},
			"a": {items: []int{2}, next: "a", hasMore: true},
		}
		var calls []string
		_, err := collectListPages(listPageOptions{PageAll: true}, fakeFetcher(loop, &calls))
		if err == nil || !strings.Contains(err.Error(), "未前进") {
			t.Fatalf("want repeated cursor error, got %v (calls=%v)", err, calls)
		}
	})

	t.Run("has_more without token stops with error", func(t *testing.T) {
		broken := map[string]fakePage{"": {items: []int{1}, hasMore: true}}
		var calls []string
		_, err := collectListPages(listPageOptions{PageAll: true}, fakeFetcher(broken, &calls))
		if err == nil {
			t.Fatal("want error for has_more without token")
		}
	})
}

func TestPrintListPageHint(t *testing.T) {
	var buf bytes.Buffer
	printListPageHint(&buf, &listPageResult[int]{HasMore: true, NextPageToken: "tok"})
	if !strings.Contains(buf.String(), "page_token=tok") || !strings.Contains(buf.String(), "--page-all") {
		t.Fatalf("hint = %q", buf.String())
	}
	buf.Reset()
	printListPageHint(&buf, &listPageResult[int]{HasMore: false})
	if buf.Len() != 0 {
		t.Fatalf("no hint expected, got %q", buf.String())
	}
	buf.Reset()
	printListPageHint(&buf, &listPageResult[int]{HasMore: true, NextPageToken: "x", Truncated: true})
	if !strings.Contains(buf.String(), "--page-limit") {
		t.Fatalf("truncated hint = %q", buf.String())
	}
}

func TestReadListPageOptionsRejectsNegativeLimit(t *testing.T) {
	c := &cobra.Command{Use: "x"}
	addListPageFlags(c)
	if err := c.Flags().Set("page-limit", "-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := readListPageOptions(c); err == nil {
		t.Fatal("want usage error for negative --page-limit")
	}
}

func TestResolveIdentityWithLegacyDefault(t *testing.T) {
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	c := &cobra.Command{Use: "x"}
	addLegacyAsFlag(c, "默认 Bot")
	c.Flags().String("user-access-token", "", "")

	legacyCalled := false
	tok, err := resolveIdentityWithLegacyDefault(c, func(*cobra.Command) (string, error) {
		legacyCalled = true
		return "legacy", nil
	})
	if err != nil || tok != "legacy" || !legacyCalled {
		t.Fatalf("without --as legacy must be used: tok=%q err=%v", tok, err)
	}

	if err := c.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}
	legacyCalled = false
	tok, err = resolveIdentityWithLegacyDefault(c, func(*cobra.Command) (string, error) {
		legacyCalled = true
		return "legacy", nil
	})
	if err != nil || tok != "" || legacyCalled {
		t.Fatalf("--as bot must force App token: tok=%q err=%v legacy=%v", tok, err, legacyCalled)
	}

	if err := c.Flags().Set("as", "nope"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveIdentityWithLegacyDefault(c, legacyBotUnlessExplicitUserToken); err == nil {
		t.Fatal("invalid --as must be rejected")
	}
}
