package cmd

import (
	"strings"
	"testing"
)

func TestResolveImageDisplaySize(t *testing.T) {
	cases := []struct {
		w, h           int
		wSet, hSet     bool
		nw, nh         int
		wantW, wantH   int
		wantErrContain string
	}{
		{w: 600, wSet: true, nw: 2048, nh: 1024, wantW: 600, wantH: 300},
		{h: 512, hSet: true, nw: 2048, nh: 1024, wantW: 1024, wantH: 512},
		{w: 300, h: 100, wSet: true, hSet: true, nw: 2048, nh: 1024, wantW: 300, wantH: 100},
		{nw: 800, nh: 600, wantW: 800, wantH: 600},
		{w: 600, wSet: true, wantErrContain: "同时提供"},
	}
	for _, c := range cases {
		w, h, err := resolveImageDisplaySize(c.w, c.h, c.wSet, c.hSet, c.nw, c.nh)
		if c.wantErrContain != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErrContain) {
				t.Errorf("%+v 期望错误含 %q，得到 %v", c, c.wantErrContain, err)
			}
			continue
		}
		if err != nil || w != c.wantW || h != c.wantH {
			t.Errorf("%+v => (%d,%d,%v)，期望 (%d,%d)", c, w, h, err, c.wantW, c.wantH)
		}
	}
}

func TestMediaInsertRejectsInvalidSizeFlags(t *testing.T) {
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")
	set := func(k, v string) { _ = docMediaInsertCmd.Flags().Set(k, v) }
	defer func() {
		for _, k := range []string{"type", "width", "height", "file"} {
			f := docMediaInsertCmd.Flags().Lookup(k)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}()
	set("file", "doc_media_insert_test.go")
	set("type", "file")
	set("width", "100")
	if err := docMediaInsertCmd.RunE(docMediaInsertCmd, []string{"doc"}); err == nil || !strings.Contains(err.Error(), "只用于 --type image") {
		t.Fatalf("--type file 搭配 --width 应报错，得到 %v", err)
	}
	set("type", "image")
	set("width", "0")
	if err := docMediaInsertCmd.RunE(docMediaInsertCmd, []string{"doc"}); err == nil || !strings.Contains(err.Error(), "--width 必须") {
		t.Fatalf("--width 0 应报错，得到 %v", err)
	}
}
