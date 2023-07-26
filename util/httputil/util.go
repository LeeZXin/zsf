package httputil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type RetryableRoundTripper struct {
	Delegated http.RoundTripper
}

func (t *RetryableRoundTripper) RoundTrip(request *http.Request) (response *http.Response, err error) {
	buf := new(bytes.Buffer)
	hasBody := request.Body != nil
	if hasBody {
		_, err = io.Copy(buf, request.Body)
		request.Body.Close()
	}
	if err != nil {
		return
	}
	for i := 0; i < 3; i++ {
		if hasBody {
			request.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		}
		response, err = t.Delegated.RoundTrip(request)
		if err == nil {
			break
		}
	}
	return
}

// NewRetryableHttpClient http client
func NewRetryableHttpClient() *http.Client {
	return &http.Client{
		Transport: &RetryableRoundTripper{
			Delegated: &http.Transport{
				TLSHandshakeTimeout: 10 * time.Second,
				MaxIdleConns:        100,
				IdleConnTimeout:     time.Minute,
				MaxConnsPerHost:     10,
			},
		},
		Timeout: 30 * time.Second,
	}
}

func Post(client *http.Client, url string, req, resp any) error {
	var (
		reqJson []byte
		err     error
	)
	if req != nil {
		reqJson, err = json.Marshal(req)
		if err != nil {
			return err
		}
	}
	post, err := client.Post(url, "application/json;charset=utf-8", bytes.NewReader(reqJson))
	if err != nil {
		return err
	}
	defer post.Body.Close()
	respBody, err := io.ReadAll(post.Body)
	if err != nil {
		return err
	}
	if resp != nil {
		return json.Unmarshal(respBody, resp)
	}
	return nil
}

func Get(client *http.Client, url string, resp any) error {
	post, err := client.Get(url)
	if err != nil {
		return err
	}
	defer post.Body.Close()
	respBody, err := io.ReadAll(post.Body)
	if err != nil {
		return err
	}
	if resp != nil {
		return json.Unmarshal(respBody, resp)
	}
	return nil
}
