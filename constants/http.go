package constants

const (
	// HttpInternalErr 是 gin context 中标记"内部错误"的 key。
	// 业务侧在错误处理时设置（见 http/ginutil 的 Error），
	// http/server 的 SentinelFilter / CircuitBreakerFilter 读取后
	// 将该请求计入熔断错误率。
	HttpInternalErr = "internal_err"
)
