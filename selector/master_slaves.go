package selector

import (
	"context"
	"errors"
	"github.com/LeeZXin/zsf/util/listutil"
)

var (
	supportedLbPolicy = []string{RoundRobinPolicy, WeightedRoundRobinPolicy}
)

type MasterSlaves[T any] struct {
	master        Node[T]
	slaves        []Node[T]
	slaveSelector Selector[T]
}

func NewMasterSlaves[T any](lbPolicy string, nodes ...Node[T]) (*MasterSlaves[T], error) {
	if !listutil.Contains[string](lbPolicy, supportedLbPolicy) {
		return nil, errors.New("unsupported lb policy")
	}
	selectorFunc, _ := FindNewSelectorFunc[T](lbPolicy)
	if len(nodes) == 0 {
		return nil, errors.New("empty nodes")
	}
	master := nodes[0]
	slaves := nodes[1:]
	var (
		slrs Selector[T]
		err  error
	)
	if len(slaves) == 0 {
		slrs = &ErrorSelector[T]{
			Err: EmptyNodesErr,
		}
	} else {
		slrs, err = selectorFunc(slaves)
		if err != nil {
			return nil, err
		}
	}
	return &MasterSlaves[T]{
		master:        master,
		slaves:        slaves,
		slaveSelector: slrs,
	}, nil
}

func (m *MasterSlaves[T]) Master() T {
	return m.master.Data
}

func (m *MasterSlaves[T]) Slaves() []T {
	ret := make([]T, 0, len(m.slaves))
	for _, slave := range m.slaves {
		ret = append(ret, slave.Data)
	}
	return ret
}

func (m *MasterSlaves[T]) SelectSlave() (T, error) {
	node, err := m.slaveSelector.Select(context.Background())
	if err != nil {
		var ret T
		return ret, err
	}
	return node.Data, nil
}

func (m *MasterSlaves[T]) IndexSlave(index int) (T, error) {
	if index >= len(m.slaves) {
		var ret T
		return ret, errors.New("index out of bound")
	}
	return m.slaves[index].Data, nil
}
