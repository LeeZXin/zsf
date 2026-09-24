/*
Package sqlitereplica 提供 SQLite 数据库的持续备份与恢复：监听数据库文件的
WAL 变更，将「快照 + WAL 段」同步到远端存储（Handler），并支持从远端恢复
出数据库文件。核心算法移植自 litestream v0.3.x（https://github.com/benbjohnson/litestream）。

# 设计概览

  - 快照：整库 LZ4 压缩直传，远端路径 <generation>/snapshots/%08x.snapshot.lz4
  - 增量：真实 WAL 逐帧校验（salt + checksum）后追加到本地 shadow WAL，
    再压缩为段上传，远端路径 <generation>/wal/%08x_%08x.wal.lz4（index_offset）
  - generation：WAL 被重置（checkpoint restart 或外部进程）时开新代，
    名字为零填充 hex 计数器（%016x），字典序最大 = 最新代
  - 恢复：最新快照 + 从快照 index 起的 WAL 段按序重放（truncate checkpoint）

# 远端存储（Handler）

file 子包提供本地目录实现（测试与单机备份），s3 子包提供 S3 协议实现
（AWS S3、阿里云 OSS S3 兼容模式、腾讯 COS、华为 OBS、MinIO 通吃，见
s3 包说明）。远端文件名格式与 litestream v0.3.x 一致，不兼容 0.4 的 LTX
格式。

# 使用方式

同步侧只有一个公开入口 Sync：一行接入，内部自动启动监听 goroutine 并
注册进程退出钩子（quit 最终钩子，保证 replica 在业务连接关闭之后才关闭，
WAL 得以保留、重启后增量续传）。参数错误直接 Fatal（fast fail）：

	sqlitereplica.Sync(sqlitereplica.SyncOptions{
		Path:    "data/sqlite3.db",
		Handler: file.NewHandler("backup-dir"),
	})

恢复在进程外进行（原库已损坏时）：Sync 要求库文件已存在，灾备恢复请用
包级 Restore，直接传 handler：

	err := sqlitereplica.Restore(ctx, h, sqlitereplica.RestoreOptions{OutputPath: restoredPath})

# 注意事项

  - 侵入性：组件会在被同步的库中创建 _zsf_replica_seq / _zsf_replica_lock
    两张辅助表，并强制开启 WAL 模式（journal_mode=wal）
  - path 指向非 sqlite 文件或空文件（如占位文件）时，组件视为「库尚未
    就绪」静默等待（Debug 日志，与文件不存在一致），文件变成合法 sqlite
    库后自动开始同步；真正损坏的库（头合法但内容坏）会持续报同步错误
  - meta 目录（默认 <dir>/.<basename>-litestream）记录了 generation、shadow
    WAL 与已上传位置（replica-pos），必须与数据库文件同生命周期。meta
    目录整体丢失无法自愈：会以同名 generation 重新开始，需人工清空远端
    前缀（或更换前缀）后再启动
  - 数据库路径含 "?" 时 DSN 拼接会破坏查询串，不支持
  - 多进程同时复制同一库不被检测（无租约机制），避免多实例指向同一远端
    前缀
  - 仅恢复最新状态，不支持时间点恢复
  - 保留策略为代级：新 generation 建立后清理旧代远端文件；代内 WAL 段不
    单独清理，随代一起删除
*/
package sqlitereplica
