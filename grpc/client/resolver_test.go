package client

import (
	"testing"

	"github.com/LeeZXin/zsf/rpc"
	"github.com/LeeZXin/zsf/services/lb"
	"google.golang.org/grpc/resolver"
)

type recordingClientConn struct {
	resolver.ClientConn
	states []resolver.State
}

func (c *recordingClientConn) UpdateState(s resolver.State) error {
	c.states = append(c.states, s)
	return nil
}

func TestUpdateStateAcceptsEmpty(t *testing.T) {
	cc := &recordingClientConn{}
	r := &customResolver{serviceName: "svc", cc: cc}
	r.updateState(nil)
	if len(cc.states) != 1 || len(cc.states[0].Addresses) != 0 {
		t.Fatalf("空实例列表也应推给 balancer，got %+v", cc.states)
	}
	r.updateState([]lb.Server{{Host: "10.0.0.1", Port: 9000, Protocol: rpc.GrpcProtocol}})
	if len(cc.states) != 2 || cc.states[1].Addresses[0].Addr != "10.0.0.1:9000" {
		t.Fatalf("got %+v", cc.states)
	}
}
