package client

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	RetryTimes = 3
)

var (
	proxy = func(*http.Request) (*url.URL, error) {
		return nil, nil
	}
)

type retryableRoundTripper struct {
	delegated http.RoundTripper
}

func (t *retryableRoundTripper) RoundTrip(request *http.Request) (response *http.Response, err error) {
	buf := new(bytes.Buffer)
	hasBody := request.Body != nil
	if hasBody {
		_, err = io.Copy(buf, request.Body)
	}
	if err != nil {
		return
	}
	for i := 0; i < RetryTimes; i++ {
		if hasBody {
			request.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		}
		response, err = t.delegated.RoundTrip(request)
		if err == nil {
			break
		}
	}
	return
}

// newRetryableHttpClient 可重试client 遇到非2xx或错误会重试
func newRetryableHttpClient() *http.Client {
	return &http.Client{
		Transport: &retryableRoundTripper{
			delegated: &http.Transport{
				Proxy:               proxy,
				TLSHandshakeTimeout: 10 * time.Second,
				MaxIdleConns:        20,
				IdleConnTimeout:     time.Minute,
			},
		},
		Timeout: 30 * time.Second,
	}
}
