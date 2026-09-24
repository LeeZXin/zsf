// Package instance 提供当前应用实例的全局身份信息（应用名、实例 ID）。
//
// 由 start 钩子（order -6，晚于 static -9）在启动时确定，进程运行期不可变：
//   - ApplicationName 优先取环境变量 APPLICATION_NAME，否则回退静态配置 application.name，
//     两者都为空则 Fatal 退出（应用名是日志、注册中心、指标标签的基础信息，缺了无法工作）
//   - ID 为随机 UUID，唯一标识当前进程实例
//
// 职责边界：只保存身份信息本身；服务注册（registry）、指标标签（promhelper）
// 等消费方自行引用本包变量。
package instance

import (
	"log"
	"os"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/utils/idutil"
)

var (
	// ApplicationName 应用名称：优先环境变量 APPLICATION_NAME，其次静态配置 application.name。
	// 用于日志、Loki 标签（service_name）、注册中心、Prometheus 标签等标识场景。
	ApplicationName string

	// ID 当前进程实例的唯一 ID（随机 UUID），用于区分同一应用的多实例部署。
	ID string

	// IsDev 是否开发模式（dev 配置为 true 时开启，如免鉴权、调试端点）。
	IsDev bool
)

const (
	DevAccount = "dev"
	DevName    = "Developer"
	DevEmail   = "dev@no.reply"
)

func init() {
	start.AddInit(func() {
		ApplicationName = os.Getenv("APPLICATION_NAME")
		if ApplicationName == "" {
			ApplicationName = static.GetString("application.name")
		}
		if ApplicationName == "" {
			log.Fatalln("application name is empty")
		}
		ID = idutil.RandomUUID()
		IsDev = static.GetBool("dev")
	}, -6)
}
