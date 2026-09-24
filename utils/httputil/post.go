// Package httputil 提供基于 JSON 的 HTTP 客户端封装，目前仅支持 POST 请求。
package httputil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/bytedance/sonic"
)

// Post 向 url 发送 JSON POST 请求（Content-Type: application/json），
// req 序列化为请求体，响应状态码非 200 时返回包含 url 与状态码的错误。
// resp 非 nil 时将响应体反序列化到 resp；resp 为 nil 时仍会排空响应体以便连接复用。
// client 为 nil 时使用 http.DefaultClient。
func Post(ctx context.Context, client *http.Client, url string, req, resp any) error {
	if client == nil {
		client = http.DefaultClient
	}
	reqBody, err := sonic.Marshal(req)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("url: %s return http status code: %d", url, response.StatusCode)
	}
	if resp != nil {
		respBody, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		err = sonic.Unmarshal(respBody, resp)
		if err != nil {
			return err
		}
	}
	return nil
}
