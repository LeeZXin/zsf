package registry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LeeZXin/zsf/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"sync"
	"time"
)

var (
	LeaseRevokedErr = errors.New("lease revoke")
)

type etcdRegistry struct {
	client *clientv3.Client
}

func (r *etcdRegistry) Register(info ServerInfo, isDown bool) (StatusChanger, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	logger.Logger.Infof("register %s, path: %s down: %v", info.GetRpcName(), info.GetRegisterPath(), isDown)
	grant, err := r.client.Grant(ctx, 10)
	if err != nil {
		cancelFunc()
		return nil, err
	}
	output, _ := json.Marshal(info.GetServer(isDown))
	_, err = r.client.Put(ctx, info.GetRegisterPath(), string(output), clientv3.WithLease(grant.ID))
	if err != nil {
		cancelFunc()
		return nil, err
	}
	return &statusChangerImpl{
		isDown:     isDown,
		info:       info,
		client:     r.client,
		leaseId:    grant.ID,
		ctx:        ctx,
		cancelFunc: cancelFunc,
	}, nil
}

type statusChangerImpl struct {
	sync.Mutex
	isDown     bool
	info       ServerInfo
	client     *clientv3.Client
	leaseId    clientv3.LeaseID
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func (r *statusChangerImpl) IsDown() bool {
	r.Lock()
	defer r.Unlock()
	return r.isDown
}

func (r *statusChangerImpl) MarkAsDown() error {
	r.Lock()
	defer r.Unlock()
	if r.isDown {
		logger.Logger.Infof("mark as down %s, path: %s: is already down", r.info.GetRpcName(), r.info.GetRegisterPath())
		return nil
	}
	logger.Logger.Infof("mark as down %s, path: %s", r.info.GetRpcName(), r.info.GetRegisterPath())
	if r.leaseId == 0 {
		return errors.New("grant first")
	}
	output, _ := json.Marshal(r.info.GetServer(true))
	_, err := r.client.Put(r.ctx, r.info.GetRegisterPath(), string(output), clientv3.WithLease(r.leaseId))
	if err == nil {
		r.isDown = true
	}
	return err
}

func (r *statusChangerImpl) MarkAsUp() error {
	r.Lock()
	defer r.Unlock()
	if !r.isDown {
		logger.Logger.Infof("mark as up %s, path: %s: is already up", r.info.GetRpcName(), r.info.GetRegisterPath())
		return nil
	}
	logger.Logger.Infof("mark as up %s, path: %s", r.info.GetRpcName(), r.info.GetRegisterPath())
	if r.leaseId == 0 {
		return errors.New("grant first")
	}
	output, _ := json.Marshal(r.info.GetServer(false))
	_, err := r.client.Put(r.ctx, r.info.GetRegisterPath(), string(output), clientv3.WithLease(r.leaseId))
	if err == nil {
		r.isDown = false
	}
	return err
}

func (r *statusChangerImpl) Deregister() error {
	r.Lock()
	defer r.Unlock()
	r.cancelFunc()
	logger.Logger.Infof("deregister %s, path: %s", r.info.GetRpcName(), r.info.GetRegisterPath())
	ctx, fn := context.WithTimeout(context.Background(), 3*time.Second)
	defer fn()
	_, err := r.client.Delete(ctx, r.info.GetRegisterPath())
	return err
}

func (r *statusChangerImpl) KeepAlive() error {
	ch, err := r.client.KeepAlive(r.ctx, r.leaseId)
	if err != nil {
		return err
	}
	defer func() {
		if r.ctx.Err() != nil {
			ctx, fn := context.WithTimeout(context.Background(), 3*time.Second)
			defer fn()
			r.client.Revoke(ctx, r.leaseId)
		}
	}()
	for {
		select {
		case res := <-ch:
			if res == nil {
				return LeaseRevokedErr
			}
		case <-r.ctx.Done():
			return r.ctx.Err()
		}
	}
}
