package mcp

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"html/template"
	"net/http"
)

//go:embed oauth_callback.html
var oauthCallbackHTML string

var oauthCallbackTemplate = template.Must(template.New("oauth_callback").Parse(oauthCallbackHTML))

type oauthCallbackPage struct {
	Status  string
	Title   string
	Service string
	Message string
	Hint    string
	Nonce   string
}

func writeOAuthCallbackPage(w http.ResponseWriter, code int, service string, status OAuthStatus) {
	page := oauthCallbackPage{
		Status: status.Status, Title: "授权失败", Service: service, Message: status.Message,
		Hint:  "可以关闭此标签页，回到 Callum 后重试。",
		Nonce: base64.RawURLEncoding.EncodeToString(randBytes()),
	}
	switch status.Status {
	case "authorized":
		page.Title = "授权成功"
		page.Hint = "可以关闭此标签页，返回 Callum 测试连接。"
	case "cancelled":
		page.Title = "已取消授权"
		if page.Message == "" {
			page.Message = "这次授权已取消。"
		}
		page.Hint = "可以关闭此标签页，返回 Callum。"
	}
	var body bytes.Buffer
	if err := oauthCallbackTemplate.Execute(&body, page); err != nil {
		http.Error(w, "无法显示授权结果，请返回 Callum。", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+page.Nonce+"'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.WriteHeader(code)
	// 浏览器关闭或网络写入失败不影响已经保存的授权结果。
	_, _ = body.WriteTo(w)
}
