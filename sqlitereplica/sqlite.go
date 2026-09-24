package sqlitereplica

/*
driverName 组件内部使用的 sqlite driver 名。

unix 平台注册时 ConnectHook 设置 SQLITE_FCNTL_PERSIST_WAL，防止最后一个
连接关闭时 WAL 文件被删除（见 sqlite_unix.go）。WAL 被删会导致 shadow
同步丢失增量（只能靠重开 generation 兜底），因此组件自己的连接必须走该
driver；恢复时 applyWAL 也用它，保证 staging WAL 的 checkpoint 行为可控。
*/
const driverName = "zsf-sqlitereplica"
