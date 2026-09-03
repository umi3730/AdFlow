package application

import (
	"context"
	"testing"

	"github.com/zhanghaiyang/adflow/internal/agentassistant/adapter/mock"
)

func TestGenerateValidatesProviderDraft(t *testing.T) {
	service := NewService(mock.NewProvider())
	draft, err := service.Generate(context.Background(), "面向二次元策略游戏活跃安卓用户，排除已经安装游戏的人")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Targeting.All) < 4 || len(draft.Targeting.None) != 1 || draft.Provider != "local-mock" {
		t.Fatalf("unexpected draft: %+v", draft)
	}
}
