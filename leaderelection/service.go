/*
Package leaderelection 基于 MySQL 行记录实现租约式选主。

每个 bizKey 在表 leader_election_lease 中对应一行，抢到锁的实例把自身标识写入
leader 并设置过期时间 expired，其他实例只能在租约过期后抢占。时间基准统一使用
MySQL 的 UNIX_TIMESTAMP(NOW(3))*1000，WHERE 比较与 SET 写入在同一条 UPDATE
语句内原子完成，避免节点间时钟偏差导致提前抢锁或双主。

用法：NewService 创建服务，Campaign 抢锁，抢到后定时 Renew 续期；Renew 返回
false 表示已失去领导权，需要重新 Campaign。进程退出时由 shutdown hook 自动
Release 租约并关闭连接。
*/
package leaderelection

import (
	"context"
	"errors"
	"time"

	"github.com/LeeZXin/zsf/leaderelection/internal"
	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"

	"xorm.io/xorm"
	"xorm.io/xorm/log"
)

// Service 租约式选主服务，内部持有 MySQL 连接与当前候选者信息。
type Service struct {
	engine    *xorm.Engine // MySQL 引擎。
	bizKey    string       // 业务键，对应一行租约记录。
	candidate string       // 候选者标识，一般为实例唯一名。
}

/*
init 建表并确保 bizKey 对应的租约行存在。
先检查再插入，行已存在时跳过，避免每次启动都浪费自增主键；INSERT IGNORE
仅兜底并发首次启动时两个实例同时插入的唯一键冲突。
*/
func (s *Service) init() error {
	err := s.engine.Sync(new(internal.Lease))
	if err != nil {
		return err
	}
	session := s.engine.NewSession()
	defer func() { _ = session.Close() }()
	exist, err := session.Where("biz_key = ?", s.bizKey).Exist(new(internal.Lease))
	if err != nil {
		return err
	}
	if !exist {
		_, err = session.Exec("INSERT IGNORE INTO "+internal.LeaseTable+" (biz_key) VALUES (?)", s.bizKey)
		return err
	}
	return nil
}

// leaseTTL 租约有效期（毫秒），持有者必须在有效期内完成下一次续期。
const leaseTTL = 60 * 1000

/*
Campaign 抢占领导权。
仅当租约已过期（expired < now）时更新成功，now 取 MySQL 时间，与 expired 的
写入在同一条 UPDATE 语句内原子完成，避免节点间时钟偏差导致提前抢锁或双主；
返回 true 表示抢锁成功。
*/
func (s *Service) Campaign() (bool, error) {
	session := s.engine.NewSession()
	defer func() { _ = session.Close() }()
	res, err := session.Exec(
		"UPDATE "+internal.LeaseTable+" SET leader = ?, expired = UNIX_TIMESTAMP(NOW(3))*1000 + ? WHERE biz_key = ? AND expired < UNIX_TIMESTAMP(NOW(3))*1000",
		s.candidate, leaseTTL, s.bizKey,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

/*
Release 释放领导权。
只有 leader 仍是自己时才清空，避免进程卡顿期间租约被他人抢占后误释放；
expired 置 0 使租约可被立即抢占。
*/
func (s *Service) Release() error {
	session := s.engine.NewSession()
	defer func() { _ = session.Close() }()
	_, err := session.Where("biz_key = ?", s.bizKey).
		And("leader = ?", s.candidate).
		MustCols("leader", "expired").
		Update(&internal.Lease{
			Leader:  "",
			Expired: 0,
		})
	return err
}

/*
Renew 续期租约，时间基准与 Campaign 一致。
条件为 leader = 自己 且 expired >= now（仍持有有效租约），续期成功返回 true；
返回 false 表示已失去领导权（租约到期被他人抢占或 leader 被改写）；
ctx 已取消时调用快速失败并返回错误。
*/
func (s *Service) Renew(ctx context.Context) (bool, error) {
	session := s.engine.NewSession().Context(ctx)
	defer func() { _ = session.Close() }()
	res, err := session.Exec(
		"UPDATE "+internal.LeaseTable+" SET expired = UNIX_TIMESTAMP(NOW(3))*1000 + ? WHERE biz_key = ? AND leader = ? AND expired >= UNIX_TIMESTAMP(NOW(3))*1000",
		leaseTTL, s.bizKey, s.candidate,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ServiceConfig 选主服务配置。
type ServiceConfig struct {
	Datasource string `json:"datasource"` // MySQL 连接串。
	BizKey     string `json:"bizKey"`     // 业务键，对应一行租约记录。
	Candidate  string `json:"candidate"`  // 候选者标识，一般为实例唯一名。
}

/*
NewService 创建选主服务。
校验必填参数与列宽限制（bizKey 最长 32 字符，candidate 最长 64 字符），初始化
引擎时区并注册 shutdown hook：退出时先 Release 租约再关闭连接，让其他实例无需
等待租约自然过期即可抢锁。
*/
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.Datasource == "" {
		return nil, errors.New("datasource name is required")
	}
	if cfg.BizKey == "" {
		return nil, errors.New("biz key is required")
	}
	if len(cfg.BizKey) > 32 {
		return nil, errors.New("biz key is too long")
	}
	if cfg.Candidate == "" {
		return nil, errors.New("candidate is required")
	}
	if len(cfg.Candidate) > 64 {
		return nil, errors.New("candidate is too long")
	}
	engine, err := xorm.NewEngine("mysql", cfg.Datasource)
	if err != nil {
		return nil, err
	}
	engine.DatabaseTZ = time.Local // 必须
	engine.TZLocation = time.Local // 必须
	engine.SetLogger(new(log.DiscardLogger))
	srv := &Service{
		engine:    engine,
		bizKey:    cfg.BizKey,
		candidate: cfg.Candidate,
	}
	quit.AddLowPriorityShutdownHook(func() {
		if err := srv.Release(); err != nil {
			logger.Logger.Error().Err(err).Msgf("failed to release lease: %v", srv.bizKey)
		}
		if err := engine.Close(); err != nil {
			logger.Logger.Error().Err(err).Msgf("failed to close engine: %v", srv.bizKey)
		}
	})
	err = srv.init()
	if err != nil {
		return nil, err
	}
	return srv, nil
}
