package service

import (
	"encoding/hex"
	"fmt"
	"gossh/app/utils"
	"gossh/gin"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const rcLocalPath = "/etc/rc.local"

type smsMessage struct {
	ID       int    `json:"id"`
	Number   string `json:"number"`
	Date     string `json:"date"`
	Content  string `json:"content"`
	RawHex   string `json:"raw_hex"`
	Tag      string `json:"tag"`
	MemStore string `json:"mem_store"`
}

func SystemSmsListHandler(c *gin.Context) {
	messages, err := loadSmsMessages()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{"messages": messages}})
}

func SystemSmsForwardHandler(c *gin.Context) {
	type body struct {
		BarkEnabled bool   `json:"bark_enabled"`
		BarkURL     string `json:"bark_url"`
		TgEnabled   bool   `json:"tg_enabled"`
		TgBotToken  string `json:"tg_bot_token"`
		TgChatID    string `json:"tg_chat_id"`
		LastID      int    `json:"last_id"`
		OnlyLatest  bool   `json:"only_latest"`
		DryRun      bool   `json:"dry_run"`
	}
	var req body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "输入数据不合法"})
		return
	}
	messages, err := loadSmsMessages()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 2, "msg": err.Error()})
		return
	}

	targets := make([]smsMessage, 0)
	for _, msg := range messages {
		if msg.ID > req.LastID {
			targets = append(targets, msg)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	if req.OnlyLatest && len(targets) > 1 {
		targets = targets[len(targets)-1:]
	}

	latestID := req.LastID
	for _, msg := range messages {
		if msg.ID > latestID {
			latestID = msg.ID
		}
	}
	if req.DryRun {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{"latest_id": latestID, "sent": 0, "messages": targets}})
		return
	}

	sent := 0
	var errs []string
	for _, msg := range targets {
		title := fmt.Sprintf("短信 %s", msg.Number)
		text := fmt.Sprintf("来自: %s\n时间: %s\n%s", msg.Number, msg.Date, msg.Content)
		if req.BarkEnabled {
			if err := sendBark(req.BarkURL, title, text); err != nil {
				errs = append(errs, "Bark: "+err.Error())
			} else {
				sent++
			}
		}
		if req.TgEnabled {
			if err := sendTelegram(req.TgBotToken, req.TgChatID, text); err != nil {
				errs = append(errs, "TG: "+err.Error())
			} else {
				sent++
			}
		}
	}
	if len(errs) > 0 {
		c.JSON(http.StatusOK, gin.H{"code": 3, "msg": strings.Join(errs, "; "), "data": gin.H{"latest_id": latestID, "sent": sent, "messages": targets}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{"latest_id": latestID, "sent": sent, "messages": targets}})
}

func SystemRcLocalGetHandler(c *gin.Context) {
	content, err := os.ReadFile(rcLocalPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "读取 rc.local 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok", "data": gin.H{"path": rcLocalPath, "content": string(content)}})
}

func SystemRcLocalSaveHandler(c *gin.Context) {
	type body struct {
		Content string `json:"content"`
	}
	var req body
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "输入数据不合法"})
		return
	}
	if len([]byte(req.Content)) > 512*1024 {
		c.JSON(http.StatusOK, gin.H{"code": 2, "msg": "rc.local 内容超过 512KB"})
		return
	}
	if err := os.WriteFile(rcLocalPath, []byte(req.Content), 0755); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 3, "msg": "保存 rc.local 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "保存成功"})
}

func loadSmsMessages() ([]smsMessage, error) {
	data, err := utils.GetDataFromUbus("zwrt_wms", "zte_libwms_get_sms_data", map[string]interface{}{
		"page":          0,
		"data_per_page": 500,
		"mem_store":     1,
		"tags":          10,
		"order_by":      "order by id desc",
	})
	if err != nil {
		return nil, fmt.Errorf("读取短信失败: %w", err)
	}
	rawMessages, ok := data["messages"].([]interface{})
	if !ok {
		return []smsMessage{}, nil
	}
	messages := make([]smsMessage, 0, len(rawMessages))
	for _, item := range rawMessages {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		rawHex := stringValue(obj["content"])
		messages = append(messages, smsMessage{
			ID:       intValue(obj["id"]),
			Number:   stringValue(obj["number"]),
			Date:     stringValue(obj["date"]),
			Content:  decodeUtf16BEHex(rawHex),
			RawHex:   rawHex,
			Tag:      stringValue(obj["tag"]),
			MemStore: stringValue(obj["mem_store"]),
		})
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID > messages[j].ID })
	return messages, nil
}

func decodeUtf16BEHex(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	bytesValue, err := hex.DecodeString(value)
	if err != nil || len(bytesValue) < 2 {
		return value
	}
	if len(bytesValue)%2 == 1 {
		bytesValue = bytesValue[:len(bytesValue)-1]
	}
	u16 := make([]uint16, 0, len(bytesValue)/2)
	for i := 0; i+1 < len(bytesValue); i += 2 {
		u16 = append(u16, uint16(bytesValue[i])<<8|uint16(bytesValue[i+1]))
	}
	return string(utf16.Decode(u16))
}

func sendBark(rawURL string, title string, body string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("Bark 地址为空")
	}
	base, err := url.Parse(rawURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("Bark 地址不合法")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + url.PathEscape(title) + "/" + url.PathEscape(body)
	return notifyGet(base.String())
}

func sendTelegram(token string, chatID string, text string) error {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	if token == "" || chatID == "" {
		return fmt.Errorf("TG Bot Token 或 Chat ID 为空")
	}
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return notifyRequest(req)
}

func notifyGet(target string) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	return notifyRequest(req)
}

func notifyRequest(req *http.Request) error {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func stringValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func intValue(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}
