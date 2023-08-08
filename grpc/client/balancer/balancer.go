package balancer

import (
	"errors"
	"github.com/LeeZXin/zsf/cmd"
	"github.com/LeeZXin/zsf/common"
	"github.com/LeeZXin/zsf/selector"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"strconv"
)

// 实现根据版本号路由请求和负载均衡

const (
	AttrKey = "grpc-attr"
)

type Attr struct {
	Weight  int    `json:"weight"`
	Version string `json:"version"`
}

type pickerBuilder struct {
	lbPolicy string
}

func (p *pickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	nodesMap := make(map[string][]selector.Node[balancer.SubConn])
	//默认版本节点先初始化
	nodesMap[common.DefaultVersion] = make([]selector.Node[balancer.SubConn], 0)
	i := 0
	for subConn, subConnInfo := range info.ReadySCs {
		weight := 1
		version := ""
		if subConnInfo.Address.Attributes.Value(AttrKey) != nil {
			attr := subConnInfo.Address.Attributes.Value(AttrKey).(Attr)
			weight = attr.Weight
			version = attr.Version
		}
		if version == "" {
			version = common.DefaultVersion
		}
		node := selector.Node[balancer.SubConn]{
			Id:     strconv.Itoa(i),
			Data:   subConn,
			Weight: weight,
		}
		nodes, ok := nodesMap[version]
		if ok {
			nodesMap[version] = append(nodes, node)
		} else {
			nodesMap[version] = append(make([]selector.Node[balancer.SubConn], 0), node)
		}
		if version != common.DefaultVersion {
			nodesMap[common.DefaultVersion] = append(nodesMap[common.DefaultVersion], node)
		}
		i += 1
	}
	selectorMap := make(map[string]selector.Selector[balancer.SubConn], len(nodesMap))
	for version, nodes := range nodesMap {
		slr, b := selector.FindNewSelectorFunc[balancer.SubConn](p.lbPolicy)
		if !b {
			return base.NewErrPicker(errors.New("unknown lbPolicy"))
		}
		st, err := slr(nodes)
		if err != nil {
			return base.NewErrPicker(err)
		}
		selectorMap[version] = st
	}
	return &picker{
		lbPolicy:    p.lbPolicy,
		selectorMap: selectorMap,
	}
}

type picker struct {
	lbPolicy    string
	selectorMap map[string]selector.Selector[balancer.SubConn]
}

func (p *picker) Pick(b balancer.PickInfo) (pickResult balancer.PickResult, err error) {
	version := cmd.GetVersion()
	var (
		nodeSelector selector.Selector[balancer.SubConn]
		ok           bool
		node         selector.Node[balancer.SubConn]
	)
	nodeSelector, ok = p.selectorMap[version]
	if !ok {
		nodeSelector = p.selectorMap[common.DefaultVersion]
	}
	node, err = nodeSelector.Select(b.Ctx)
	if err != nil {
		return
	}
	pickResult = balancer.PickResult{
		SubConn: node.Data.(balancer.SubConn),
	}
	return
}
