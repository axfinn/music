// Package mail 封装第三方发信服务（t.jaxiu.cn）的 OAuth2 客户端凭据流程。
//
// 协议：
//  1. POST /oauth/token  Basic(client_id:client_secret) + grant_type=client_credentials
//     → access_token（Bearer，本项目实测 3600 秒有效）
//  2. POST /send         Authorization: Bearer <token>
//     → {"from":..., "to":[...], "subject":..., "body":...}
//
// token 在进程内缓存并提前刷新，避免每封验证码邮件都换一次 token。
// 发件人 from 必须落在业务的发件人白名单内，否则上游返回 403 from_not_allowed；
// 这里不重复校验白名单（白名单在服务端），只保证配置里的 from 被如实传递。
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/axfinn/music/backend/config"
)

// ErrNotConfigured 邮件服务未配齐（缺 client_id / client_secret）。
var ErrNotConfigured = errors.New("邮件服务未配置：请设置 MAIL_CLIENT_ID 与 MAIL_CLIENT_SECRET")

// Client 发信客户端，并发安全。
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	from         string
	fromName     string
	http         *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewClient 由配置构造客户端。缺凭据时仍返回实例，但 Send 会返回 ErrNotConfigured，
// 让上层能给出明确错误而不是启动时直接崩。
func NewClient(cfg *config.MailConfig) *Client {
	timeout := time.Duration(cfg.TimeoutSecond) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		from:         cfg.From,
		fromName:     cfg.FromName,
		http:         &http.Client{Timeout: timeout},
	}
}

// Configured 是否已配齐凭据。
func (c *Client) Configured() bool {
	return c != nil && c.clientID != "" && c.clientSecret != ""
}

// From 当前配置的发件人地址（供自检接口展示，方便运维核对白名单）。
func (c *Client) From() string {
	if c == nil {
		return ""
	}
	return c.from
}

// Send 发送一封纯文本邮件。
func (c *Client) Send(ctx context.Context, to []string, subject, body string) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	if len(to) == 0 {
		return errors.New("收件人不能为空")
	}
	if strings.TrimSpace(c.from) == "" {
		return errors.New("未配置发件人地址（MAIL_FROM）")
	}

	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"from":    c.from,
		"to":      to,
		"subject": subject,
		"body":    body,
	}
	if c.fromName != "" {
		payload["from_name"] = c.fromName
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/send", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("发信请求失败: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("发信失败(%d): %s", res.StatusCode, upstreamError(respBody))
	}
	return nil
}

// token 返回缓存的 access_token，过期前 5 分钟主动刷新。
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	body := strings.NewReader("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth/token", body)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接邮件服务失败: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("邮件服务鉴权失败(%d): %s", res.StatusCode, upstreamError(raw))
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("解析邮件服务响应失败: %w", err)
	}
	if resp.AccessToken == "" {
		return "", errors.New("邮件服务未返回 access_token")
	}

	ttl := time.Duration(resp.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	// 留 5 分钟安全边界；对于超短 token，至少提前 1/10 刷新。
	buffer := 5 * time.Minute
	if ttl/10 < buffer {
		buffer = ttl / 10
	}
	c.accessToken = resp.AccessToken
	c.tokenExpiry = time.Now().Add(ttl - buffer)
	return c.accessToken, nil
}

// upstreamError 从上游 JSON 错误体里提取可读信息，兼容 error_description / message。
func upstreamError(raw []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"error_description", "error", "message", "msg"} {
			if v, ok := obj[key].(string); ok && v != "" {
				return v
			}
		}
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "空响应"
	}
	// 上游 nginx 的 502/504 会回一整页 HTML，直接塞进错误文案里既难读又占屏。
	// 识别成常见的网关错误并给简短说明。
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		for _, code := range []string{"502", "503", "504", "500"} {
			if strings.Contains(s, code+" Bad Gateway") || strings.Contains(s, code+" Service Unavailable") ||
				strings.Contains(s, code+" Gateway Time-out") || strings.Contains(s, code+" Internal Server Error") {
				return "邮件服务网关错误(" + code + ")，请稍后重试"
			}
		}
		return "邮件服务返回异常页面，请稍后重试"
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
