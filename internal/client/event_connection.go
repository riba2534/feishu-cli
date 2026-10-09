package client

import (
	"encoding/json"
	"fmt"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// eventConnectionCheckTimeout 远端长连接探测是 consume 启动前的 best-effort 预检，不能拖慢启动。
const eventConnectionCheckTimeout = 5 * time.Second

// CountEventConnections 查询当前 App 在飞书侧已建立的事件长连接数量
// （GET /open-apis/event/v1/connection，Bot 身份，对齐官方 consume/remote_preflight.go）。
//
// 返回值 > 0 表示已有其他进程/机器以同一 App 连着长连接：飞书会把事件随机分发到各连接，
// 新起的 consume 只能收到其中一部分。调用方据此告警；探测失败不应阻断启动。
func CountEventConnections() (int, error) {
	cli, err := GetClient()
	if err != nil {
		return 0, err
	}
	resp, err := cli.Get(ContextWithTimeout(eventConnectionCheckTimeout), "/open-apis/event/v1/connection", nil, larkcore.AccessTokenTypeTenant)
	if err != nil {
		return 0, fmt.Errorf("查询事件长连接数失败: %w", err)
	}
	if err := CheckAPIResponse("查询事件长连接数", resp); err != nil {
		return 0, err
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			OnlineInstanceCnt int `json:"online_instance_cnt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &env); err != nil {
		return 0, fmt.Errorf("查询事件长连接数失败: 解析响应失败: %w", err)
	}
	return env.Data.OnlineInstanceCnt, nil
}
