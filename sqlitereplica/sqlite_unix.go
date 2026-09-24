//go:build unix

package sqlitereplica

import (
	"database/sql"
	"fmt"

	"github.com/LeeZXin/zsf/start"

	sqlite3 "github.com/mattn/go-sqlite3"
)

func init() {
	start.AddInit(func() {
		sql.Register(driverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.SetFileControlInt("main", sqlite3.SQLITE_FCNTL_PERSIST_WAL, 1); err != nil {
					return fmt.Errorf("cannot set file control: %w", err)
				}
				return nil
			},
		})
	}, -7)
}
