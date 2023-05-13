package balancer

import (
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/selector"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"strconv"
)

// 实现根据版本号路由请求和负载均衡

const (
	ClientAttrKey = "grpc-attr"
)

type GrpcAttr struct {
	Weight  int    `json:"weight"`
	Version string `json:"version"`
}

type pickerBuilder struct {
	lbPolicy selector.LbPolicy
}

func (p *pickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	mn := make(map[string][]*selector.Node)
	//默认版本节点先初始化
	mn[common.DefaultVersion] = make([]*selector.Node, 0)
	i := 0
	for c, ci := range info.ReadySCs {
		weight := 1
		version := ""
		if ci.Address.Attributes.Value(ClientAttrKey) != nil {
			attr := ci.Address.Attributes.Value(ClientAttrKey).(GrpcAttr)
			weight = attr.Weight
			version = attr.Version
		}
		if version == "" {
			version = common.DefaultVersion
		}
		n := &selector.Node{
			Id:     strconv.Itoa(i),
			Data:   c,
			Weight: weight,
		}
		ns, ok := mn[version]
		if ok {
			mn[version] = append(ns, n)
		} else {
			mn[version] = append(make([]*selector.Node, 0), n)
		}
		if version != common.DefaultVersion {
			mn[common.DefaultVersion] = append(mn[common.DefaultVersion], n)
		}
		i += 1
	}
	ms := make(map[string]selector.Selector, len(mn))
	for ver, ns := range mn {
		st := selector.NewSelectorFuncMap[p.lbPolicy](ns)
		err := st.Init()
		if err != nil {
			return base.NewErrPicker(err)
		}
		ms[ver] = st
	}
	return &picker{
		lbPolicy: p.lbPolicy,
		ms:       ms,
	}
}

type picker struct {
	lbPolicy selector.LbPolicy
	ms       map[string]selector.Selector
}

func (p *picker) Pick(b balancer.PickInfo) (balancer.PickResult, error) {
	version := common.Version
	robinSelector, ok := p.ms[version]
	if ok {
		node, err := robinSelector.Select()
		if err != nil {
			return balancer.PickResult{}, err
		}
		return balancer.PickResult{SubConn: node.Data.(balancer.SubConn)}, nil
	} else {
		node, err := p.ms[common.DefaultVersion].Select()
		if err != nil {
			return balancer.PickResult{}, err
		}
		return balancer.PickResult{SubConn: node.Data.(balancer.SubConn)}, nil
	}
}
