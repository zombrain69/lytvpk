package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 真机复现：面板被反向代理挡下时返回 500 + 886 字节 HTML，
// 上传任务的 error 字段里塞进了整页 <html>…</html>。
func TestDoPanelUploadRequestCompressesHTMLErrorPage(t *testing.T) {
	html := "<html><head><title>500 Internal Server Error</title></head><body>" +
		strings.Repeat("<!-- padding -->\n", 50) + "</body></html>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/upload/init", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = doPanelUploadRequest(request, nil)
	if err == nil {
		t.Fatal("500 响应应当返回错误")
	}
	message := err.Error()
	if !strings.Contains(message, "面板请求失败(500)") {
		t.Fatalf("缺少状态码说明：%q", message)
	}
	if !strings.Contains(message, "HTML 错误页") {
		t.Fatalf("应给出一句可读的中文说明：%q", message)
	}
	if strings.Contains(message, "<html") || strings.Contains(message, "<!--") {
		t.Fatalf("不该把 HTML 正文塞进提示：%q", message)
	}
	if len([]rune(message)) > 200 {
		t.Fatalf("提示太长（%d 字）：%q", len([]rune(message)), message)
	}
}

func TestDescribePanelErrorBody(t *testing.T) {
	if got := describePanelErrorBody(""); got != "" {
		t.Fatalf("空正文应返回空串，实际 %q", got)
	}

	html := describePanelErrorBody("<!DOCTYPE html><html><body>boom</body></html>")
	if !strings.Contains(html, "HTML 错误页") {
		t.Fatalf("HTML 应换成说明，实际 %q", html)
	}

	plain := describePanelErrorBody("  upload   failed\n\nbecause: disk full  ")
	if plain != "upload failed because: disk full" {
		t.Fatalf("纯文本应折掉多余空白，实际 %q", plain)
	}

	long := describePanelErrorBody(strings.Repeat("x", 500))
	if len([]rune(long)) != 161 || !strings.HasSuffix(long, "…") {
		t.Fatalf("超长正文应截断到 160 字加省略号，实际 %d 字", len([]rune(long)))
	}
}
