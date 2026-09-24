package leaderelection

import (
	"context"
	"time"

	"github.com/LeeZXin/zsf/logger"
	"github.com/LeeZXin/zsf/quit"
)

/*
Do 阻塞式运行选主任务：循环竞选领导权，当选后启动续期协程并同步执行 fn。
fn 必须依赖 ctx.Done 判断是否失去领导权，只有续期失败（失去领导权）时才返回；
fn 提前返回时先取消续期再 Release 租约，其他实例无需等待 TTL 即可抢锁。
竞选失败 30 秒后重试；进程退出（quit.Stopping）时取消 ctx、释放租约并返回。
*/
func Do(cfg ServiceConfig, fn func(context.Context)) {
	srv, err := NewService(cfg)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("create leader election service failed")
	}
	for {
		select {
		case <-quit.Stopping():
			return
		default:
		}
		ok, err := srv.Campaign()
		if err == nil && ok {
			ctx, cancelFunc := context.WithCancel(context.Background())
			go func() {
				select {
				case <-quit.Stopping():
					cancelFunc()
				case <-ctx.Done():
				}
			}()
			go func() {
				ticker := time.NewTicker(20 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						success, err := srv.Renew(ctx)
						// ctx 已取消（fn 提前返回后的收尾）属于正常退出，不算错误
						if err != nil && ctx.Err() == nil {
							logger.Logger.Error().Err(err).Msgf("failed to renew biz_key: %s leader: %s", cfg.BizKey, cfg.Candidate)
						}
						if err != nil || !success {
							cancelFunc()
							return
						}
					}
				}
			}()
			// fn必须依赖ctx.Done来判断是否已失去领导权
			fn(ctx)
			// fn 返回后取消 ctx，让续期协程尽快退出，避免僵尸续期
			cancelFunc()
			if err := srv.Release(); err != nil {
				logger.Logger.Error().Err(err).Msgf("failed to release biz_key: %s leader: %s", cfg.BizKey, cfg.Candidate)
			}
		} else if err != nil {
			logger.Logger.Error().Err(err).Msgf("failed to campaign biz_key: %s candidate: %s", cfg.BizKey, cfg.Candidate)
		}
		select {
		case <-quit.Stopping():
			return
		case <-time.After(30 * time.Second):
		}
	}
}
