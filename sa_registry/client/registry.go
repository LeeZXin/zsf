package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/util/httputil"
	"io"
	"net/http"
	"strconv"
)

const (
	jsonContentType = "application/json;charset=utf-8"
)

type RegistryClient struct {
	http  *http.Client
	host  string
	token string
}

func NewRegistryClient(host, token string) *RegistryClient {
	return &RegistryClient{
		http:  httputil.NewRetryableHttpClient(),
		host:  host,
		token: token,
	}
}

// RegisterService 注册服务
func (c *RegistryClient) RegisterService(ctx context.Context, reqDTO RegisterServiceReqDTO) error {
	var respDTO BaseResp
	if err := c.post(ctx, "/registry/registerService", reqDTO, &respDTO); err != nil {
		return err
	}
	if respDTO.Code != 0 {
		return errors.New(respDTO.Message)
	}
	return nil
}

// DeregisterService 注销服务
func (c *RegistryClient) DeregisterService(ctx context.Context, reqDTO DeregisterServiceReqDTO) error {
	var respDTO BaseResp
	if err := c.post(ctx, "/registry/deregisterService", reqDTO, &respDTO); err != nil {
		return err
	}
	if respDTO.Code != 0 {
		return errors.New(respDTO.Message)
	}
	return nil
}

// PassTTL 心跳续期
func (c *RegistryClient) PassTTL(ctx context.Context, reqDTO PassTtlReqDTO) error {
	var respDTO BaseResp
	if err := c.post(ctx, "/registry/passTtl", reqDTO, &respDTO); err != nil {
		return err
	}
	if respDTO.Code != 0 {
		return errors.New(respDTO.Message)
	}
	return nil
}

// GetServiceInfoList 获取服务列表
func (c *RegistryClient) GetServiceInfoList(ctx context.Context, serviceName string) ([]ServiceInfoDTO, error) {
	var respDTO GetServiceInfoListRespDTO
	reqDTO := GetServiceInfoListReqDTO{
		ServiceName: serviceName,
	}
	if err := c.post(ctx, "/registry/getServiceInfoList", reqDTO, &respDTO); err != nil {
		return nil, err
	}
	if respDTO.Code != 0 {
		return nil, errors.New(respDTO.Message)
	}
	return respDTO.ServiceList, nil
}

// post 发送post请求
func (c *RegistryClient) post(ctx context.Context, url string, reqDTO, respDTO any) error {
	url = "http://" + c.host + url
	bodyBytes, err := json.Marshal(reqDTO)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", jsonContentType)
	req.Header.Set("z-token", c.token)
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("http returns statusCode:" + strconv.Itoa(response.StatusCode))
	}
	bodyBytes, err = io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bodyBytes, &respDTO)
	if err != nil {
		return err
	}
	return nil
}
