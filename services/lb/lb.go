package lb

import (
	"context"
)

type Server struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Version string `json:"version"`
	Region  string `json:"region"`
	Zone    string `json:"zone"`
}

func (s *Server) IsSameAs(s2 Server) bool {
	return s.Name == s2.Name &&
		s.Host == s2.Host &&
		s.Port == s2.Port &&
		s.Version == s2.Version &&
		s.Weight == s2.Weight
}

type LoadBalancer interface {
	SetServers([]Server)
	ChooseServer(context.Context) (Server, error)
}

type Policy string

const (
	RoundRobin       Policy = "round_robin"
	WeightRoundRobin Policy = "weighted_round_robin"
)
