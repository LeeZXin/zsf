package registry

import (
	"context"
	"encoding/json"
	"github.com/LeeZXin/zsf/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

type etcdRegistry struct {
	client *clientv3.Client
}

func (r *etcdRegistry) RegisterSelf(info RegisterInfo) DeregisterAction {
	ctx, cancelFunc := context.WithCancel(context.Background())
	logger.Logger.Infof("register %s, path: %s", info.GetRpcName(), info.GetRegisterPath())
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			// 一直续约
			err := r.grantAndKeepalive(ctx, info)
			if err != nil && err != context.Canceled {
				logger.Logger.Error(err)
			}
			// 续约异常 重新注册
			time.Sleep(10 * time.Second)
		}
	}()
	return DeregisterAction(cancelFunc)
}

func (r *etcdRegistry) grantAndKeepalive(ctx context.Context, info RegisterInfo) error {
	grant, err := r.client.Grant(ctx, 10)
	if err != nil {
		return err
	}
	path := info.GetRegisterPath()
	output, _ := json.Marshal(info.GetServiceAddr())
	_, err = r.client.Put(ctx, path, string(output), clientv3.WithLease(grant.ID))
	if err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err = r.client.KeepAlive(ctx, grant.ID)
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
	}
}
