package server

const (
	SuccessCode = 0
	// InvalidParamsCode 参数错误
	InvalidParamsCode = 100001
	// UnauthorizedCode 未授权
	UnauthorizedCode = 100002
	// ExecuteFailCode 执行错误
	ExecuteFailCode = 100003
)

var (
	DefaultSuccessResp = BaseResp{
		Code:    SuccessCode,
		Message: "success",
	}
	DefaultFailBindArgResp = BaseResp{
		Code:    InvalidParamsCode,
		Message: "args error",
	}
)

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
