package cmd

import (
	"strings"
	"testing"
)

func TestSlidesCmdRegistered(t *testing.T) {
	for _, child := range rootCmd.Commands() {
		if child.Name() == "slides" {
			subs := child.Commands()
			if len(subs) < 2 {
				t.Fatalf("slides 子命令数量不足: got %d, want ≥ 2", len(subs))
			}
			have := map[string]bool{}
			for _, c := range subs {
				have[c.Name()] = true
			}
			for _, want := range []string{"create", "media-upload"} {
				if !have[want] {
					t.Errorf("slides 子命令缺少 %q", want)
				}
			}
			return
		}
	}
	t.Fatal("slides 顶层命令未注册到 rootCmd")
}

func TestSlidesCreateFlags(t *testing.T) {
	if slidesCreateCmd.Flag("title") == nil {
		t.Error("slides create 缺少 --title flag")
	}
	if slidesCreateCmd.Flag("width") == nil {
		t.Error("slides create 缺少 --width flag")
	}
	if slidesCreateCmd.Flag("height") == nil {
		t.Error("slides create 缺少 --height flag")
	}
	if !strings.Contains(slidesCreateCmd.Short, "Slides") {
		t.Errorf("slides create Short 文案缺少 Slides: %q", slidesCreateCmd.Short)
	}
}

func TestSlidesMediaUploadFlags(t *testing.T) {
	if slidesMediaUploadCmd.Flag("file") == nil {
		t.Error("slides media-upload 缺少 --file flag")
	}
	if slidesMediaUploadCmd.Flag("presentation-token") == nil {
		t.Error("slides media-upload 缺少 --presentation-token flag")
	}
}
