package registry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

var (
	leaseFailedErr = errors.New("lease failed")
)

type etcdRegistry struct {
	client *clientv3.Client
}

func (r *etcdRegistry) RegisterSelf(info ServerInfo) DeregisterAction {
	ctx, cancelFunc := context.WithCancel(context.Background())
	logger.Logger.Infof("register %s, path: %s", info.GetRpcName(), info.GetRegisterPath())
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			// 一直续约
			err := r.grantAndKeepalive(ctx, info)
			if err == nil || ctx.Err() != nil {
				return
			}
			logger.Logger.Error(err)
			// 续约异常 重新注册
			time.Sleep(5 * time.Second)
			logger.Logger.Infof("try to re-register %s, path: %s", info.GetRpcName(), info.GetRegisterPath())
		}
	}()
	return func() {
		cancelFunc()
		logger.Logger.Infof("deregister %s, path: %s", info.GetRpcName(), info.GetRegisterPath())
		timeoutCtx, timeoutFunc := context.WithTimeout(context.Background(), 2*time.Second)
		defer timeoutFunc()
		_, err := r.client.Delete(timeoutCtx, info.GetRegisterPath())
		if err != nil {
			logger.Logger.Error(err)
		}
	}
}

func (r *etcdRegistry) grantAndKeepalive(ctx context.Context, info ServerInfo) error {
	grant, err := r.client.Grant(ctx, 10)
	if err != nil {
		return err
	}
	output, _ := json.Marshal(info.GetServer())
	_, err = r.client.Put(ctx, info.GetRegisterPath(), string(output), clientv3.WithLease(grant.ID))
	if err != nil {
		return err
	}
	ch, err := r.client.KeepAlive(ctx, grant.ID)
	if err != nil {
		return err
	}
	defer func() {
		if ctx.Err() != nil {
			// 如果是cancelFunc触发的 手动释放租约
			_, err := r.client.Revoke(ctx, grant.ID)
			if err != nil {
				logger.Logger.Error(err)
			}
		}
	}()
	for {
		select {
		case res := <-ch:
			if res == nil {
				//续约终止
				return leaseFailedErr
			}
		case <-ctx.Done():
			return nil
		}
	}
}
