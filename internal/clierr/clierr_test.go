package clierr

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapKeepsMessageAndChain(t *testing.T) {
	base := errors.New("原始错误")
	err := Auth(base)
	if err.Error() != "原始错误" {
		t.Fatalf("打标签不应改变错误文本，得到 %q", err.Error())
	}
	if !errors.Is(err, base) {
		t.Fatal("errors.Is 应能穿透分类包装")
	}
	if Wrap(KindAuth, nil) != nil {
		t.Fatal("nil 错误包装后应仍为 nil")
	}
}

func TestKindsWalksWrappedAndJoined(t *testing.T) {
	inner := Network(errors.New("dial tcp: i/o timeout"))
	outer := fmt.Errorf("刷新失败: %w", Auth(inner))
	got := Kinds(outer)
	if len(got) != 2 || got[0] != KindAuth || got[1] != KindNetwork {
		t.Fatalf("Kinds 应按外层到内层返回 [auth network]，得到 %v", got)
	}

	joined := errors.Join(errors.New("a"), Usagef("bad flag"))
	if !HasKind(joined, KindUsage) {
		t.Fatal("errors.Join 的分支也应被识别")
	}
	if HasKind(errors.New("plain"), KindUsage) {
		t.Fatal("未标注的错误不应有分类")
	}
}

func TestKindExitCodes(t *testing.T) {
	cases := map[Kind]int{
		KindGeneral:              ExitGeneral,
		KindUsage:                ExitUsage,
		KindAuth:                 ExitAuth,
		KindNetwork:              ExitNetwork,
		KindConfirmationRequired: ExitConfirmationRequired,
		KindCancelled:            ExitGeneral,
	}
	for kind, want := range cases {
		if got := kind.ExitCode(); got != want {
			t.Errorf("%s.ExitCode() = %d, want %d", kind, got, want)
		}
	}
}
