package httputil

import (
	"bytes"
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
