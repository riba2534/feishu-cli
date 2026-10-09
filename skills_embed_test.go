package main

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/skillbundle"
)

// TestEmbeddedSkillsMatchRepository 保证 go:embed 白名单没有漏掉任何可分发文件：
// 内嵌内容与仓库 skills/ 目录经同一分发过滤后的哈希必须完全一致。
// 新增技能顶层目录（references 之外）或以 _ 开头的文件时，本测试会失败，提示更新白名单。
func TestEmbeddedSkillsMatchRepository(t *testing.T) {
	sub, err := fs.Sub(embeddedSkills, "skills")
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := skillbundle.Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := skillbundle.Load(os.DirFS("skills"))
	if err != nil {
		t.Fatal(err)
	}
	if embedded.Hash == onDisk.Hash {
		return
	}
	for _, s := range onDisk.Skills() {
		e, ok := embedded.Skill(s.Name)
		if !ok {
			t.Errorf("技能 %s 未被内嵌", s.Name)
			continue
		}
		embeddedFiles := e.FileHashes()
		for _, f := range s.Files {
			if got, ok := embeddedFiles[f.Path]; !ok {
				t.Errorf("%s/%s 未被内嵌（检查 skills_embed.go 的 go:embed 白名单）", s.Name, f.Path)
			} else if got != f.SHA256 {
				t.Errorf("%s/%s 内嵌内容与磁盘不一致", s.Name, f.Path)
			}
		}
	}
	t.Fatalf("内嵌技能哈希 %s 与仓库 %s 不一致", embedded.Hash, onDisk.Hash)
}

// TestEmbeddedSkillsExcludeNonDistributable 评测、触发评测集与测试脚本不应进入分发内容。
func TestEmbeddedSkillsExcludeNonDistributable(t *testing.T) {
	err := fs.WalkDir(embeddedSkills, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(p, "/evals/") || strings.HasSuffix(p, "/evals") || strings.Contains(p, "trigger-") || strings.Contains(p, "__pycache__") {
			t.Errorf("不应内嵌: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
