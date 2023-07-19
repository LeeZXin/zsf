package apigw

import (
	"errors"
	"github.com/LeeZXin/zsf/apigw/hexpr"
	"github.com/LeeZXin/zsf/selector"
	"github.com/LeeZXin/zsf/util/httputil"
	"github.com/gin-gonic/gin"
	"net/http"
)

// 路由策略
const (
	// FullMatchType 全匹配
	FullMatchType = "fullMatch"
	// PrefixMatchType 前缀匹配
	PrefixMatchType = "prefixMatch"
	// ExprMatchType 表达式匹配
	ExprMatchType = "exprMatch"

	// MockJsonType json格式
	MockJsonType = "json"
	// MockStringType string格式
	MockStringType = "string"
)

var (
	putTransportFuncMap = map[string]PutTransportFunc{
		FullMatchType: func(routers *Routers, config RouterConfig, transport *Transport) error {
			if config.Path == "" {
				return errors.New("empty path")
			}
			routers.putFullMatchTransport(config.Path, transport)
			return nil
		},
		PrefixMatchType: func(routers *Routers, config RouterConfig, transport *Transport) error {
			if config.Path == "" {
				return errors.New("empty path")
			}
			routers.putPrefixMatchTransport(config.Path, transport)
			return nil
		},
		ExprMatchType: func(routers *Routers, config RouterConfig, transport *Transport) error {
			expr, err := hexpr.BuildExpr(config.Expr)
			if err != nil {
				return err
			}
			routers.putExprMatchTransport(expr, transport)
			return nil
		},
	}
)

type Target struct {
	Weight int    `json:"weight"`
	Target string `json:"target"`
}

type PutTransportFunc func(*Routers, RouterConfig, *Transport) error

type MockContent struct {
	ContentType string `json:"contentType"`
	StatusCode  int    `json:"statusCode"`
	RespStr     string `json:"respStr"`
}

// RouterConfig 路由配置信息
type RouterConfig struct {
	// MatchType 匹配模式
	MatchType string `json:"matchType"`
	// Path url path
	Path string `json:"path"`
	// Expr 表达式
	Expr hexpr.PlainInfo `json:"expr"`
	// ServiceName 服务名称 用于服务发现
	ServiceName string `json:"serviceName"`
	// Targets 转发目标 配置权重信息
	Targets []Target `json:"targets"`
	// TargetType 转发目标类型 服务发现或ip域名转发
	TargetType string `json:"targetType"`
	// TargetLbPolicy 负载均衡策略
	TargetLbPolicy string `json:"targetLbPolicy"`
	// RewriteType 路径重写类型
	RewriteType string `json:"rewriteType"`
	// ReplacePath 路径完全覆盖path
	ReplacePath string `json:"replacePath"`
	// MockContent mock数据
	MockContent MockContent `json:"mockContent"`
	// Extra 附加信息
	Extra map[string]any
}

func (r *RouterConfig) FillDefaultVal() {
	if r.TargetLbPolicy == "" {
		r.TargetLbPolicy = selector.RoundRobinPolicy
	}
	if r.RewriteType == "" {
		r.RewriteType = CopyFullPathRewriteType
	}
}
func (r *RouterConfig) Validate() error {
	if r.MatchType == "" {
		return errors.New("empty match type")
	} else {
		_, ok := putTransportFuncMap[r.MatchType]
		if !ok {
			return errors.New("wrong matchType")
		}
	}
	if r.MatchType == ExprMatchType {
		if err := r.Expr.Validate(); err != nil {
			return err
		}
	} else {
		if r.Path == "" {
			return errors.New("empty path")
		}
	}
	if r.TargetType == "" {
		return errors.New("empty target type")
	} else {
		_, ok := newTargetFuncMap[r.TargetType]
		if !ok {
			return errors.New("wrong target type")
		}
		if r.TargetType == DomainTargetType {
			if r.Targets == nil || len(r.Targets) == 0 {
				return errors.New("empty target")
			}
		}
		_, ok = selector.NewSelectorFuncMap[r.TargetLbPolicy]
		if !ok {
			return errors.New("wrong lb policy")
		}
	}
	if r.RewriteType == "" {
		return errors.New("empty RewriteType")
	} else {
		_, ok := rewriteStrategyFuncMap[r.RewriteType]
		if !ok {
			return errors.New("wrong RewriteType")
		}
	}
	return nil
}

type Routers struct {
	//精确匹配
	fullMatch map[string]*Transport
	//前缀匹配
	prefixMatch *Trie
	//表达式匹配
	exprMatch map[*hexpr.Expr]*Transport
	//连接池
	httpClient *http.Client
}

func NewRouters(httpClient *http.Client) *Routers {
	if httpClient == nil {
		httpClient = httputil.NewRetryableHttpClient()
	}
	return &Routers{
		httpClient: httpClient,
	}
}

func (r *Routers) putFullMatchTransport(path any, trans *Transport) {
	if r.fullMatch == nil {
		r.fullMatch = make(map[string]*Transport, 8)
	}
	r.fullMatch[path.(string)] = trans
}

func (r *Routers) putPrefixMatchTransport(path any, transport *Transport) {
	if r.prefixMatch == nil {
		r.prefixMatch = &Trie{}
	}
	r.prefixMatch.Insert(path.(string), transport)
}

func (r *Routers) putExprMatchTransport(expr any, trans *Transport) {
	if r.exprMatch == nil {
		r.exprMatch = make(map[*hexpr.Expr]*Transport, 8)
	}
	r.exprMatch[expr.(*hexpr.Expr)] = trans
}

func (r *Routers) FindTransport(c *gin.Context) (*Transport, bool) {
	path := c.Request.URL.Path
	if r.fullMatch != nil {
		//精确匹配
		trans, ok := r.fullMatch[path]
		if ok {
			return trans, true
		}
	}
	if r.prefixMatch != nil {
		//前缀匹配
		node, ok := r.prefixMatch.PrefixSearch(path, LongestMatchType)
		if ok {
			return node.data.(*Transport), true
		}
	}
	if r.exprMatch != nil {
		//表达式匹配
		exprMap := r.exprMatch
		for expr, tr := range exprMap {
			if expr.Execute(c) {
				t := tr
				return t, true
			}
		}
	}
	return nil, false
}

// AddRouter 添加路由转发
func (r *Routers) AddRouter(config RouterConfig) error {
	var rewrite RewriteStrategy
	if config.TargetType != MockTargetType {
		strategyFunc, ok := rewriteStrategyFuncMap[config.RewriteType]
		if !ok {
			return errors.New("wrong rewrite type")
		}
		rewrite = strategyFunc(config)
	}
	targetFunc, ok := newTargetFuncMap[config.TargetType]
	if !ok {
		return errors.New("wrong target type")
	}
	st, rpc, err := targetFunc(config, r.httpClient)
	if err != nil {
		return err
	}
	extra := config.Extra
	if extra == nil {
		extra = make(map[string]any)
	}
	trans := &Transport{
		Extra:           extra,
		rewriteStrategy: rewrite,
		targetSelector:  st,
		rpcExecutor:     rpc,
	}
	transportFunc, ok := putTransportFuncMap[config.MatchType]
	if !ok {
		return errors.New("path match type")
	}
	err = transportFunc(r, config, trans)
	if err != nil {
		return err
	}
	return nil
}
