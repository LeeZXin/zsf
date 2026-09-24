//go:build windows

package sqlitereplica

import (
	"database/sql"

	"github.com/LeeZXin/zsf/start"

	sqlite3 "github.com/mattn/go-sqlite3"
)

/*
init windows 平台不支持 SQLITE_FCNTL_PERSIST_WAL，退化为普通 driver 注册。
WAL 保活语义缺失，本组件在 windows 上的同步能力不保证。
*/
func init() {
	start.AddInit(func() {
		sql.Register(driverName, &sqlite3.SQLiteDriver{})
	}, -7)
}
