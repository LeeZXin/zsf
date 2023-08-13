package httputil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	JsonContentType = "application/json;charset=utf-8"
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

func Post(client *http.Client, url string, header map[string]string, req, resp any) error {
	var (
		reqJson []byte
		err     error
	)
	if req != nil {
		reqJson, err = json.Marshal(req)
		if err != nil {
			return err
		}
	} else {
		reqJson = []byte{}
	}
	request, err := http.NewRequest("POST", url, bytes.NewReader(reqJson))
	if err != nil {
		return err
	}
	if header != nil {
		for k, v := range header {
			request.Header.Set(k, v)
		}
	}
	request.Header.Set("Content-Type", JsonContentType)
	post, err := client.Do(request)
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

func Get(client *http.Client, url string, header map[string]string, resp any) error {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if header != nil {
		for k, v := range header {
			request.Header.Set(k, v)
		}
	}
	get, err := client.Do(request)
	if err != nil {
		return err
	}
	defer get.Body.Close()
	respBody, err := io.ReadAll(get.Body)
	if err != nil {
		return err
	}
	if resp != nil {
		return json.Unmarshal(respBody, resp)
	}
	return nil
}
