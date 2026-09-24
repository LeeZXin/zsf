package database

import (
	"context"

	"github.com/LeeZXin/zsf/config/static"
	"github.com/LeeZXin/zsf/env"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/xormdb"
)

// Sync 同步（创建/更新）模型对应的表结构，一般在包的 init 中调用。
// 仅在 SIT 环境（env.IsSitEnv）或 sqlite3 驱动下执行，生产 mysql 环境为
// 空操作——避免生产环境被意外改表；执行失败直接 Fatal。
// models 为 xorm 模型结构体（实现 TableName 或带 xorm tag）。
func Sync(models ...any) {
	if env.IsSitEnv() || env.IsDebugEnv() || static.GetString("xorm.driver") == xormdb.Sqlite3 {
		session := NewSession(context.Background())
		defer session.Close()
		err := session.Sync(models...)
		if err != nil {
			logger.Logger.Fatal().Err(err).Msg("sync models failed")
		}
	}
}

// DropTables 删除已废弃的表。环境门控与 Sync 相同：仅 SIT / debug / sqlite3。
// 表不存在则跳过；删除失败直接 Fatal。
func DropTables(names ...string) {
	if len(names) == 0 {
		return
	}
	if env.IsSitEnv() || env.IsDebugEnv() || static.GetString("xorm.driver") == xormdb.Sqlite3 {
		beans := make([]any, len(names))
		for i, name := range names {
			beans[i] = name
		}
		if err := GetEngine().DropTables(beans...); err != nil {
			logger.Logger.Fatal().Err(err).Msg("drop tables failed")
		}
	}
}

// Run 获取一次数据库会话，执行 fn 后关闭会话；fn 返回错误时直接 Fatal。
// 适用于一次性脚本/初始化逻辑；注意这是非事务语义（需要事务请用 WithTx）。
func Run(fn func(context.Context) error) {
	ctx, closer := Context(context.Background())
	err := fn(ctx)
	closer.Close()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("run session failed")
	}
}
