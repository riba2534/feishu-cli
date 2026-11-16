package client

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/riba2534/feishu-cli/internal/safefile"
)

type ChatSyncOptions struct {
	Name           string   `json:"name"`
	FromChatID     string   `json:"from_chat_id"`
	MessageIDs     []string `json:"message_ids,omitempty"`
	LastMessages   int      `json:"last_messages,omitempty"`
	SessionFile    string   `json:"web_session_file"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type ChatSyncMessage struct {
	ID             string `json:"id"`
	ChatID         string `json:"chat_native_id"`
	SenderID       string `json:"sender_native_id"`
	CreateTime     uint64 `json:"create_time"`
	CreateTimeMS   uint64 `json:"create_time_ms"`
	Type           uint64 `json:"type"`
	Position       uint64 `json:"position"`
	Text           string `json:"text,omitempty"`
	Content        []byte `json:"-"`
	originalSender string
	batchCopy      bool
}

type ChatSyncPlan struct {
	Name           string            `json:"name"`
	FromChatID     string            `json:"from_chat_id"`
	OwnerID        string            `json:"owner_native_id"`
	MemberIDs      []string          `json:"member_native_ids"`
	MemberNames    map[string]string `json:"member_names"`
	Messages       []ChatSyncMessage `json:"messages"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type ChatSyncResult struct {
	NativeChatID string   `json:"native_chat_id"`
	Name         string   `json:"name"`
	OwnerID      string   `json:"owner_native_id"`
	MessageIDs   []string `json:"synced_native_message_ids"`
	Verified     bool     `json:"verified"`
	Replayed     bool     `json:"replayed"`
}

type nativeSession struct {
	Region     string            `json:"region"`
	UserID     string            `json:"user_id"`
	Cookies    map[string]string `json:"cookies"`
	AppID      string            `json:"web_app_id"`
	baseURL    string
	httpClient *http.Client
}

func ValidateChatSyncOptions(options ChatSyncOptions) error {
	if length := len([]rune(options.Name)); strings.TrimSpace(options.Name) == "" || length > 60 {
		return clierr.Usagef("--name 不能为空且最多 60 个字符")
	}
	if !nativeID(options.FromChatID) {
		return clierr.Usagef("--from-chat-id 需要网页端数字会话 ID，不接受 oc_ ID")
	}
	if (len(options.MessageIDs) > 0) == (options.LastMessages != 0) {
		return clierr.Usagef("--message-ids 与 --last-messages 必须且只能指定一个")
	}
	if options.LastMessages < 0 || options.LastMessages > 50 || len(options.MessageIDs) > 50 {
		return clierr.Usagef("同步消息数量必须为 1 到 50")
	}
	seen := map[string]bool{}
	for _, messageID := range options.MessageIDs {
		if !nativeID(messageID) || seen[messageID] {
			return clierr.Usagef("--message-ids 需要不重复的网页端数字消息 ID，不接受 om_ ID")
		}
		seen[messageID] = true
	}
	if strings.TrimSpace(options.IdempotencyKey) == "" || len([]rune(options.IdempotencyKey)) > 50 {
		return clierr.Usagef("--idempotency-key 不能为空且最多 50 个字符")
	}
	return safefile.ValidateInputPath(options.SessionFile)
}

func nativeID(value string) bool {
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0
}

func loadNativeSession(path string) (*nativeSession, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, safefile.InputFileError(path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, clierr.Usagef("网页登录态必须是非符号链接的普通 JSON 文件，大小最多 1 MB")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, clierr.Usagef("网页登录态权限必须为 0600，请先执行 chmod 600")
	}
	data, err := safefile.ReadInputFile(path)
	if err != nil {
		return nil, err
	}
	var session nativeSession
	if json.Unmarshal(data, &session) != nil {
		return nil, clierr.Usagef("网页登录态 JSON 无效；未输出凭据内容")
	}
	if session.Region == "" {
		session.Region = "feishu"
	}
	domain := "feishu.cn"
	switch session.Region {
	case "feishu":
	case "lark":
		domain = "larksuite.com"
	default:
		return nil, clierr.Usagef("网页登录态 region 仅支持 feishu 或 lark")
	}
	if !nativeID(session.UserID) || session.Cookies["session"] == "" {
		return nil, clierr.Authf("缺少网页用户身份或 session；OAuth auth login 不能替代网页登录态")
	}
	session.baseURL = "https://internal-api-lark-api." + domain
	return &session, nil
}

func (session *nativeSession) request(method, path string, body []byte, command uint64) ([]byte, error) {
	request, err := http.NewRequestWithContext(Context(), method, session.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造网页请求失败")
	}
	for name, value := range session.Cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 feishu-cli")
	request.Header.Set("X-Source", "web")
	request.Header.Set("X-Web-Version", "3.9.32")
	request.Header.Set("X-Lgw-Os-Type", "1")
	request.Header.Set("X-Lgw-Terminal-Type", "2")
	if command != 0 {
		request.Header.Set("Content-Type", "application/x-protobuf")
		request.Header.Set("X-Command", strconv.FormatUint(command, 10))
		request.Header.Set("X-Command-Version", "5.7.0")
		appID := session.AppID
		if appID == "" {
			appID = "161471"
		}
		request.Header.Set("X-Appid", appID)
	} else {
		request.Header.Set("X-App-Id", "12")
		request.Header.Set("X-Api-Version", "1.0.8")
		request.Header.Set("X-Csrf-Token", session.Cookies["swp_csrf_token"])
		request.Header.Set("X-Device-Info", "platform=websdk")
	}
	controlled := session.httpClient
	if controlled == nil {
		controlled = rawHTTPClient()
	}
	httpClient := *controlled
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, clierr.Network(fmt.Errorf("网页请求失败；未输出 Cookie 或底层请求详情"))
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, clierr.Authf("网页请求 HTTP %d，请核对登录态与设备访问权限；不自动重登或切换身份", response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("网页请求 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if err != nil || len(data) > 8<<20 {
		return nil, fmt.Errorf("网页响应读取失败或超过 8 MB")
	}
	return data, nil
}

func (session *nativeSession) verifyIdentity() error {
	data, err := session.request(http.MethodGet, "/accounts/web/user?app_id=12", nil, 0)
	if err != nil {
		return err
	}
	var response struct {
		Data struct {
			User struct {
				ID json.Number `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &response) != nil || response.Data.User.ID.String() != session.UserID {
		return clierr.Authf("网页登录身份与登录态文件 user_id 不一致；未执行建群")
	}
	return nil
}

func (session *nativeSession) gateway(command uint64, body []byte) (nativeFields, error) {
	cid, err := newNativeCID()
	if err != nil {
		return nil, err
	}
	packet := nativeNumber(nil, 2, 1)
	packet = nativeNumber(packet, 3, command)
	packet = nativeBytes(packet, 5, body)
	packet = nativeString(packet, 6, cid)
	data, err := session.request(http.MethodPost, "/im/gateway/", packet, command)
	if err != nil {
		return nil, err
	}
	response, err := parseNative(data)
	if err != nil {
		return nil, err
	}
	if response.number(4) != 0 || len(response.bytes(5)) == 0 {
		return nil, fmt.Errorf("网页网关未返回成功数据，status=%d", response.number(4))
	}
	return parseNative(response.bytes(5))
}

func newNativeCID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("无法生成网页请求 ID: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func (session *nativeSession) chat(chatID string) (nativeFields, error) {
	response, err := session.gateway(64, nativeString(nil, 1, chatID))
	if err != nil {
		return nil, err
	}
	chats, err := nativeMapValues(response, 1)
	if err != nil {
		return nil, err
	}
	for _, chat := range chats {
		if chat.text(1) == chatID {
			return chat, nil
		}
	}
	return nil, fmt.Errorf("网页登录用户不可见该会话")
}

func decodeSyncMessage(fields nativeFields) ChatSyncMessage {
	message := ChatSyncMessage{ID: fields.text(1), Type: fields.number(2), SenderID: fields.text(3),
		ChatID: fields.text(10), CreateTime: fields.number(4), Content: fields.bytes(5), Position: fields.number(13),
		CreateTimeMS: fields.number(62), originalSender: fields.text(47), batchCopy: fields.number(53) != 0}
	if message.Type == 4 {
		if content, err := parseNative(message.Content); err == nil {
			message.Text = content.text(1)
			if richText, err := parseNative(content.bytes(3)); err == nil && richText.text(2) != "" {
				message.Text = richText.text(2)
			}
		}
	}
	return message
}

func (session *nativeSession) messages(command uint64, body []byte) ([]ChatSyncMessage, error) {
	response, err := session.gateway(command, body)
	if err != nil {
		return nil, err
	}
	values, err := nativeMapValues(response, 1)
	if err != nil {
		return nil, err
	}
	var messages []ChatSyncMessage
	for _, fields := range values {
		messages = append(messages, decodeSyncMessage(fields))
	}
	sort.Slice(messages, func(left, right int) bool { return messages[left].Position < messages[right].Position })
	return messages, nil
}

func PrepareChatSync(options ChatSyncOptions) (*ChatSyncPlan, error) {
	if err := ValidateChatSyncOptions(options); err != nil {
		return nil, err
	}
	session, err := loadNativeSession(options.SessionFile)
	if err != nil {
		return nil, err
	}
	return session.prepareChatSync(options)
}

func (session *nativeSession) prepareChatSync(options ChatSyncOptions) (*ChatSyncPlan, error) {
	if err := session.verifyIdentity(); err != nil {
		return nil, err
	}
	chat, err := session.chat(options.FromChatID)
	if err != nil {
		return nil, err
	}
	if chat.number(2) != 1 {
		return nil, clierr.Usagef("原生同步建群仅支持从双人私聊发起")
	}
	membersBody := nativeString(nil, 1, options.FromChatID)
	membersBody = nativeNumber(membersBody, 2, 1)
	membersBody = nativeNumber(membersBody, 5, 3)
	members, err := session.gateway(44, membersBody)
	if err != nil {
		return nil, err
	}
	chatters, err := nativeMapValues(members, 1)
	if err != nil {
		return nil, err
	}
	plan := &ChatSyncPlan{Name: options.Name, FromChatID: options.FromChatID, OwnerID: session.UserID,
		IdempotencyKey: options.IdempotencyKey, MemberNames: map[string]string{}}
	for _, chatter := range chatters {
		if !nativeID(chatter.text(1)) {
			return nil, fmt.Errorf("私聊成员 ID 无效，未执行建群")
		}
		plan.MemberIDs = append(plan.MemberIDs, chatter.text(1))
		plan.MemberNames[chatter.text(1)] = chatter.text(2)
	}
	sort.Strings(plan.MemberIDs)
	if len(plan.MemberIDs) != 2 || plan.MemberIDs[0] == plan.MemberIDs[1] || (plan.MemberIDs[0] != session.UserID && plan.MemberIDs[1] != session.UserID) {
		return nil, fmt.Errorf("无法核对私聊双方成员，未执行建群")
	}
	var command uint64 = 8
	var body []byte
	if len(options.MessageIDs) > 0 {
		for _, messageID := range options.MessageIDs {
			body = nativeString(body, 1, messageID)
		}
	} else {
		command = 43
		body = nativeString(body, 1, options.FromChatID)
		last := chat.number(15)
		if last >= 1<<31 {
			return nil, fmt.Errorf("来源会话消息位置无效")
		}
		start := uint64(0)
		if last >= 99 {
			start = last - 99
		}
		for position := start; position <= last; position++ {
			body = nativeNumber(body, 2, position)
		}
	}
	messages, err := session.messages(command, body)
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, messageID := range options.MessageIDs {
		requested[messageID] = true
	}
	seen := map[string]bool{}
	for _, message := range messages {
		if message.ChatID != options.FromChatID {
			return nil, fmt.Errorf("源消息不属于指定私聊，未执行建群")
		}
		if message.Type == 6 || len(message.Content) == 0 {
			continue
		}
		if !nativeID(message.ID) || seen[message.ID] || (len(requested) > 0 && !requested[message.ID]) {
			return nil, fmt.Errorf("源消息 ID 与选择范围不一致")
		}
		if message.SenderID != plan.MemberIDs[0] && message.SenderID != plan.MemberIDs[1] {
			return nil, fmt.Errorf("源消息发送者与私聊双方不一致，未执行建群")
		}
		seen[message.ID] = true
		plan.Messages = append(plan.Messages, message)
	}
	if options.LastMessages > 0 {
		if len(plan.Messages) < options.LastMessages {
			return nil, fmt.Errorf("最近 100 个消息位置中不足 %d 条可同步消息", options.LastMessages)
		}
		plan.Messages = plan.Messages[len(plan.Messages)-options.LastMessages:]
	} else if len(plan.Messages) != len(options.MessageIDs) {
		return nil, fmt.Errorf("所选消息包含不可见、系统或已撤回消息，未执行建群")
	}
	return plan, nil
}

func sameSyncMessage(source, destination ChatSyncMessage) bool {
	sender := destination.originalSender
	if sender == "" {
		sender = destination.SenderID
	}
	if source.SenderID != sender || source.Type != destination.Type || source.CreateTime != destination.CreateTime || source.CreateTimeMS != destination.CreateTimeMS || destination.batchCopy {
		return false
	}
	if source.Type == 4 && source.Text != "" {
		return source.Text == destination.Text
	}
	return bytes.Equal(source.Content, destination.Content)
}

type chatSyncState struct {
	Fingerprint string          `json:"fingerprint"`
	Status      string          `json:"status"`
	CID         string          `json:"cid"`
	Result      *ChatSyncResult `json:"result,omitempty"`
}

func CreateChatWithHistory(options ChatSyncOptions, plan *ChatSyncPlan) (*ChatSyncResult, error) {
	if err := ValidateChatSyncOptions(options); err != nil {
		return nil, err
	}
	session, err := loadNativeSession(options.SessionFile)
	if err != nil {
		return nil, err
	}
	return session.createChatWithHistory(options, plan)
}

func syncPlanFingerprint(region string, plan *ChatSyncPlan) string {
	encoded, _ := json.Marshal(plan)
	digest := sha256.New()
	digest.Write([]byte(region))
	digest.Write(encoded)
	if plan != nil {
		for _, message := range plan.Messages {
			content := sha256.Sum256(message.Content)
			digest.Write(content[:])
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func (session *nativeSession) createChatWithHistory(options ChatSyncOptions, plan *ChatSyncPlan) (*ChatSyncResult, error) {
	if plan == nil || plan.OwnerID != session.UserID {
		return nil, fmt.Errorf("同步计划与网页登录用户不一致")
	}
	freshPlan, err := session.prepareChatSync(options)
	if err != nil {
		return nil, err
	}
	fingerprint := syncPlanFingerprint(session.Region, plan)
	if fingerprint != syncPlanFingerprint(session.Region, freshPlan) {
		return nil, fmt.Errorf("参数、成员或源消息与确认计划不一致，未执行建群；请重新预览")
	}
	directory, err := profile.ActiveDir()
	if err != nil {
		return nil, err
	}
	directory = filepath.Join(directory, "native-chat-sync")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return nil, fmt.Errorf("回执目录必须是非符号链接的私有目录（0700）: %s", directory)
	}
	key := sha256.Sum256([]byte(options.IdempotencyKey))
	statePath := filepath.Join(directory, hex.EncodeToString(key[:])+".json")
	lock, err := os.OpenFile(statePath+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("同一请求正在执行或上次异常退出，请先核对群与回执，不自动移除锁: %s", statePath)
	}
	defer func() {
		lock.Close()
		os.Remove(statePath + ".lock")
	}()
	cid, err := newNativeCID()
	if err != nil {
		return nil, err
	}
	state := chatSyncState{Fingerprint: fingerprint, CID: cid, Status: "prepared"}
	if info, statErr := os.Lstat(statePath); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() > 1<<20 || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
			return nil, fmt.Errorf("已有回执必须是私有的普通 JSON 文件（0600），禁止重建")
		}
		data, readErr := os.ReadFile(statePath)
		if readErr != nil {
			return nil, readErr
		}
		if json.Unmarshal(data, &state) != nil || state.Fingerprint != fingerprint {
			return nil, fmt.Errorf("同一幂等键的参数、用户或源消息已变化，禁止重复建群")
		}
		if state.Result == nil {
			return nil, fmt.Errorf("上次建群结果未知，请先核对群列表与回执，不自动重建: %s", statePath)
		}
		if !nativeID(state.Result.NativeChatID) {
			return nil, fmt.Errorf("已有回执群 ID 无效，禁止重建")
		}
		state.Result.Replayed = true
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	save := func() error {
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		return safefile.AtomicWriteFileTrusted(statePath, data, 0o600)
	}
	if state.Result == nil {
		state.Status = "request_sent"
		if err := save(); err != nil {
			return nil, err
		}
		body := nativeNumber(nil, 1, 2)
		body = nativeString(body, 3, plan.Name)
		for _, memberID := range plan.MemberIDs {
			body = nativeString(body, 6, memberID)
		}
		body = nativeString(body, 8, plan.FromChatID)
		for _, message := range plan.Messages {
			body = nativeString(body, 9, message.ID)
		}
		body = nativeNumber(body, 14, 1)
		body = nativeString(body, 16, state.CID)
		response, err := session.gateway(13, body)
		if err != nil {
			return nil, fmt.Errorf("群可能已创建，禁止自动重试或换键重建；先查群列表和回执: %s", statePath)
		}
		chat, err := parseNative(response.bytes(1))
		if err != nil || !nativeID(chat.text(1)) {
			return nil, fmt.Errorf("建群响应缺少有效群 ID，禁止重建；回执: %s", statePath)
		}
		state.Result = &ChatSyncResult{NativeChatID: chat.text(1), Name: chat.text(4), OwnerID: chat.text(6)}
		state.Status = "created"
		if err := save(); err != nil {
			return state.Result, fmt.Errorf("群已创建但回执保存失败，不要重建: %s", state.Result.NativeChatID)
		}
	}
	chat, err := session.chat(state.Result.NativeChatID)
	state.Result.Verified = false
	if err != nil {
		return state.Result, err
	}
	if chat.number(2) != 2 || chat.text(4) != plan.Name || chat.text(6) != plan.OwnerID {
		return state.Result, fmt.Errorf("群已创建，但名称或群主回读不一致；不补发、不重建")
	}
	membersBody := nativeString(nil, 1, state.Result.NativeChatID)
	membersBody = nativeNumber(membersBody, 2, 1)
	membersBody = nativeNumber(membersBody, 5, 100)
	members, err := session.gateway(44, membersBody)
	if err != nil {
		return state.Result, err
	}
	chatters, err := nativeMapValues(members, 1)
	if err != nil {
		return state.Result, err
	}
	memberIDs := map[string]bool{}
	for _, chatter := range chatters {
		memberIDs[chatter.text(1)] = true
	}
	for _, memberID := range plan.MemberIDs {
		if !memberIDs[memberID] {
			return state.Result, fmt.Errorf("群已创建，但未核对到私聊双方成员；不重建、不自动邀请")
		}
	}
	body := nativeString(nil, 1, state.Result.NativeChatID)
	limit := chat.number(15)
	if limit > uint64(len(plan.Messages)+10) {
		limit = uint64(len(plan.Messages) + 10)
	}
	for position := uint64(0); position <= limit; position++ {
		body = nativeNumber(body, 2, position)
	}
	messages, err := session.messages(43, body)
	if err != nil {
		return state.Result, err
	}
	state.Result.MessageIDs = nil
	for _, source := range plan.Messages {
		var matches []string
		for _, destination := range messages {
			if destination.ChatID == state.Result.NativeChatID && sameSyncMessage(source, destination) {
				matches = append(matches, destination.ID)
			}
		}
		if len(matches) != 1 {
			return state.Result, fmt.Errorf("群已创建，但同步消息的正文、原作者或原时间未唯一匹配；不补发、不重建")
		}
		state.Result.MessageIDs = append(state.Result.MessageIDs, matches[0])
	}
	state.Result.Verified = true
	state.Status = "verified"
	return state.Result, save()
}
