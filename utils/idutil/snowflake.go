/*
Package idutil 提供 ID 生成工具：
  - Snowflake 分布式 ID（int64，趋势递增），及追加随机尾号的字符串 ID（防枚举）
  - 随机 UUID：RandomUUID 无连字符（32 位十六进制）；RawUUID 带连字符（Claude session-id）

Snowflake 节点 ID 在 init 时确定：优先读取配置 snowflake.node，
未配置时随机选择 0~1023 之间的值（多实例部署时注意碰撞概率）。
*/
package idutil

import (
	"log"
	"math/rand/v2"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/start"
	"github.com/LeeZXin/zsf/utils/strutil"

	"github.com/bwmarrin/snowflake"
)

var (
	sf   *snowflake.Node
	node int64
)

/*
init 注册 Snowflake 节点初始化钩子（order -6，依赖 static -9）。

Node ID 的确定顺序：
 1. 配置文件中 snowflake.node 的值（若配置且 >= 0）
 2. 否则随机选择 0~1023 之间的一个值

Snowflake 算法需要全局唯一的 Node ID，冲突会导致 ID 重复。
在 k8s 环境下，每个 Pod 随机选择 Node ID，碰撞概率为 1/1024（可接受）。
若需要严格不冲突，应通过配置文件为每个实例分配不同的 snowflake.node。

生成的 ID 结构（bwmarrin/snowflake，共 64 位）：

	最高位 0 + 41 位毫秒时间戳 + 10 位节点 ID + 12 位序列号

ID 趋势递增（在同一毫秒内序列号递增），适合作为数据库主键（B+树索引友好）。
*/
func init() {
	start.AddInit(func() {
		key := "snowflake.node"
		if static.Exists(key) {
			node = static.GetInt64(key)
			if node < 0 {
				node = rand.Int64N(1024)
			}
		} else {
			node = rand.Int64N(1024)
		}
		var err error
		sf, err = snowflake.NewNode(node)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}, -6)
}

/*
GenPlusSnowflakeId 生成 Snowflake ID 的字符串形式，并追加 4 位随机数字。

返回格式: 雪花ID（19位十进制数字） + 4 位随机数字 = 23 位数字字符串。

追加随机数字的目的是增加 ID 的随机性，防止攻击者通过 ID 推测数据规模。
*/
func GenPlusSnowflakeId() string {
	return sf.Generate().String() + strutil.RandomNumStr(4)
}

/*
GenSnowflakeId 生成原始 Snowflake ID（int64）。

用于需要整数 ID 的场景（如数据库自增主键替代方案）。
*/
func GenSnowflakeId() int64 {
	return sf.Generate().Int64()
}
