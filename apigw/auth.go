package apigw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/spf13/cast"
	"net/http"
	"strings"
	"time"
)

const (
	JsonContentType = "application/json;charset=utf-8"
	ContentTypeTag  = "Content-Type"
)

type UriType string

const (
	HttpUriType      UriType = "http"
	DiscoveryUriType UriType = "discovery"
)

type ArgLocation string

const (
	QueryLocation  ArgLocation = "query"
	HeaderLocation ArgLocation = "header"
)

type AuthFunc func(*ApiContext) bool

type AuthUri struct {
	Address         string `json:"address"`
	DiscoveryTarget string `json:"discoveryTarget"`
	Path            string `json:"path"`
	Timeout         int    `json:"timeout"`
}

type AuthParameter struct {
	TargetName     string      `json:"targetName"`
	TargetLocation ArgLocation `json:"targetLocation"`
	SourceName     string      `json:"sourceName"`
	SourceLocation ArgLocation `json:"sourceLocation"`
}

type AuthConfig struct {
	Id                    string          `json:"id"`
	UriType               UriType         `json:"uriType"`
	Uri                   AuthUri         `json:"uri"`
	Parameters            []AuthParameter `json:"parameters"`
	ErrorMessage          string          `json:"errorMessage"`
	ErrorStatusCode       int             `json:"errorStatusCode"`
	PassThroughHeaderList []string        `json:"passThroughHeaderList"`
}

func (c *AuthConfig) Validate() error {
	switch c.UriType {
	case HttpUriType:
		if c.Uri.Address == "" {
			return errors.New("empty uri address")
		}
	case DiscoveryUriType:
		if c.Uri.DiscoveryTarget == "" {
			return errors.New("empty discovery type")
		}
	default:
		return errors.New("wrong uri type")
	}
	return nil
}

func (t *httpExecutor) defaultAuth(c *ApiContext) bool {
	config, b := c.config.AuthConfig.(AuthConfig)
	if !b {
		c.String(http.StatusInternalServerError, "")
		return false
	}
	path := config.Uri.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	authReqJson, authReqHeader := getAuthRequestBody(c, config)
	var url string
	switch config.UriType {
	case HttpUriType:
		url = config.Uri.Address + path
	case DiscoveryUriType:
		server, err := discovery.ChooseServer(c, config.Uri.DiscoveryTarget)
		if err != nil {
			c.String(config.ErrorStatusCode, config.ErrorMessage)
			return false
		}
		url = "http://" + fmt.Sprintf("%s:%d", server.Host, server.Port) + path
	}
	reqBody, _ := json.Marshal(authReqJson)
	var (
		authReq *http.Request
		err     error
	)
	if config.Uri.Timeout > 0 {
		timeout, cancelFunc := context.WithTimeout(c, time.Duration(config.Uri.Timeout)*time.Second)
		defer cancelFunc()
		authReq, err = http.NewRequestWithContext(timeout, http.MethodPost, url, bytes.NewReader(reqBody))
	} else {
		authReq, err = http.NewRequestWithContext(c, http.MethodPost, url, bytes.NewReader(reqBody))
	}
	if err != nil {
		c.String(config.ErrorStatusCode, config.ErrorMessage)
		return false
	}
	for k := range c.Request.Header {
		authReq.Header.Set(k, c.Request.Header.Get(k))
	}
	for k, v := range authReqHeader {
		authReq.Header.Set(k, v)
	}
	authReq.Header.Set(ContentTypeTag, JsonContentType)
	resp, err := t.httpClient.Do(authReq)
	if err != nil {
		c.String(config.ErrorStatusCode, config.ErrorMessage)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.String(config.ErrorStatusCode, config.ErrorMessage)
		return false
	}
	for _, k := range config.PassThroughHeaderList {
		v := resp.Header.Get(k)
		if v != "" {
			c.header.Set(k, v)
		}
	}
	return true
}

func parseRequestBody2Map(c *ApiContext) map[string]any {
	if strings.Contains(c.Request.Header.Get(ContentTypeTag), "application/json") {
		body := c.reqBody
		if body == nil {
			return map[string]any{}
		}
		ret := make(map[string]any)
		json.Unmarshal(body, &ret)
		return ret
	} else {
		query := make(map[string]string)
		err := c.ShouldBindQuery(&query)
		if err != nil {
			return map[string]any{}
		}
		ret := make(map[string]any, len(query))
		for k, v := range query {
			ret[k] = v
		}
		return ret
	}
}

func getAuthRequestBody(c *ApiContext, config AuthConfig) (map[string]any, map[string]string) {
	arr := config.Parameters
	requestJson := parseRequestBody2Map(c)
	req := make(map[string]any)
	header := make(map[string]string)
	for _, p := range arr {
		switch p.SourceLocation {
		case HeaderLocation:
			switch p.TargetLocation {
			case QueryLocation:
				req[p.TargetName] = c.Request.Header.Get(p.SourceName)
			case HeaderLocation:
				header[p.TargetName] = c.Request.Header.Get(p.SourceName)
			}
		case QueryLocation:
			t, b := requestJson[p.SourceName]
			if !b {
				break
			}
			switch p.TargetLocation {
			case QueryLocation:
				req[p.TargetName] = t
			case HeaderLocation:
				header[p.TargetName] = cast.ToString(t)
			}
		}
	}
	return req, header
}
