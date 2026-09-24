// Package constants 存放框架内跨包共享的常量定义（无函数与状态）。
//
// 约定：
//   - 与配置相关的路径、字段名等全局约定统一在此登记，避免各包各自硬编码
//   - gin context 的 key 使用自定义类型而非字符串，防止与其他包冲突；
//     仅字符串 key（如 HttpInternalErr）作为内部约定使用时，限制在本框架包内
package constants

const (
	// ResourcesDir 是程序工作目录下资源根目录（resources），
	// 静态配置 yaml、HTTPS 证书等均相对于该目录定位。
	ResourcesDir = "resources"
)
