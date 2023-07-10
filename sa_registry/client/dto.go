package client

type DeregisterServiceReqDTO struct {
	ServiceName string `json:"serviceName"`
	InstanceId  string `json:"instanceId"`
}

type PassTtlReqDTO struct {
	ServiceName string `json:"serviceName"`
	InstanceId  string `json:"instanceId"`
}

type RegisterServiceReqDTO struct {
	ServiceName   string `json:"serviceName"`
	Ip            string `json:"ip"`
	Port          int    `json:"port"`
	InstanceId    string `json:"instanceId"`
	Weight        int    `json:"weight"`
	Version       string `json:"version"`
	LeaseDuration int    `json:"leaseDuration"`
}

// BaseResp 通用基础resp
type BaseResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type GetServiceInfoListReqDTO struct {
	ServiceName string `json:"serviceName"`
}

type ServiceInfoDTO struct {
	ServiceName   string `json:"serviceName"`
	Ip            string `json:"ip"`
	Port          int    `json:"port"`
	InstanceId    string `json:"instanceId"`
	Weight        int    `json:"weight"`
	Version       string `json:"version"`
	LeaseDuration int    `json:"leaseDuration"`
}

type GetServiceInfoListRespDTO struct {
	BaseResp
	ServiceList []ServiceInfoDTO `json:"serviceList"`
}
