package registry

//服务发现
//目前只实现consul

// IRegistry 服务发现接口
type IRegistry interface {
	StartRegisterSelf()
}

// ServiceRegistryConfig 注册所需的信息
type ServiceRegistryConfig struct {
	// ApplicationName 应用名称
	ApplicationName string
	// Ip ip地址
	Ip string
	// Port 端口
	Port int
	// Scheme 服务协议
	Scheme string
	// Weight 权重
	Weight int
}

func RegisterSelf(config ServiceRegistryConfig) {
	r := ConsulRegistry{
		Config: config,
	}
	r.StartRegisterSelf()
}
