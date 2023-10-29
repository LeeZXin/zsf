package apigw

import (
	"errors"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/listutil"
	"github.com/LeeZXin/zsf-utils/selector"
	"github.com/LeeZXin/zsf-utils/trieutil"
	"github.com/LeeZXin/zsf/apigw/hexpr"
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

type Target struct {
	Weight int    `json:"weight"`
	Target string `json:"target"`
}

type PutTransportFunc func(*Routers, RouterConfig, *transportImpl) error

type MockContent struct {
	Headers     string `json:"headers"`
	ContentType string `json:"contentType"`
	StatusCode  int    `json:"statusCode"`
	RespStr     string `json:"respStr"`
}

// RouterConfig 路由配置信息
type RouterConfig struct {
	Id string `json:"id"`
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
	// AuthConfig 鉴权配置
	AuthConfig AuthConfig `json:"authConfig"`
	// 是否需要鉴权
	NeedAuth bool
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
	}
	if ok := listutil.Contains(r.MatchType, []string{
		FullMatchType, PrefixMatchType, ExprMatchType,
	}); !ok {
		return errors.New("wrong matchType")
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
	if ok := listutil.Contains(r.TargetType, []string{
		DiscoveryTargetType, DomainTargetType, MockTargetType,
	}); !ok {
		return errors.New("wrong target type")
	}
	if r.TargetType == DomainTargetType {
		if r.Targets == nil || len(r.Targets) == 0 {
			return errors.New("empty target")
		}
	}
	if r.TargetType != MockTargetType {
		if ok := listutil.Contains(r.TargetLbPolicy, []string{
			selector.RoundRobinPolicy, selector.WeightedRoundRobinPolicy,
		}); !ok {
			return errors.New("wrong lb policy")
		}
		if ok := listutil.Contains(r.RewriteType, []string{
			CopyFullPathRewriteType, StripPrefixRewriteType, ReplaceAnyRewriteType,
		}); !ok {
			return errors.New("wrong RewriteType")
		}
	}
	if r.NeedAuth {
		return r.AuthConfig.Validate()
	}
	return nil
}

type Routers struct {
	//精确匹配
	fullMatch map[string]Transport
	//前缀匹配
	prefixMatch *trieutil.Trie[Transport]
	//表达式匹配
	exprMatch map[*hexpr.Expr]Transport
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

func (r *Routers) putFullMatchTransport(path string, trans Transport) {
	if r.fullMatch == nil {
		r.fullMatch = make(map[string]Transport, 8)
	}
	r.fullMatch[path] = trans
}

func (r *Routers) putPrefixMatchTransport(path string, transport Transport) {
	if r.prefixMatch == nil {
		r.prefixMatch = &trieutil.Trie[Transport]{}
	}
	r.prefixMatch.Insert(path, transport)
}

func (r *Routers) putExprMatchTransport(expr *hexpr.Expr, trans Transport) {
	if r.exprMatch == nil {
		r.exprMatch = make(map[*hexpr.Expr]Transport, 8)
	}
	r.exprMatch[expr] = trans
}

func (r *Routers) FindTransport(c *gin.Context) (Transport, bool) {
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
		node, ok := r.prefixMatch.PrefixSearch(path, trieutil.LongestMatchType)
		if ok {
			return node, true
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
	err := config.Validate()
	if err != nil {
		return err
	}
	var rewrite RewriteStrategy
	if config.TargetType != MockTargetType {
		switch config.RewriteType {
		case CopyFullPathRewriteType:
			rewrite = copyFullPathStrategy(config)
		case ReplaceAnyRewriteType:
			rewrite = replaceAnyStrategy(config)
		case StripPrefixRewriteType:
			rewrite = stripPrefixStrategy(config)
		}
	}
	var (
		hs  hostSelector
		rpc rpcExecutor
	)
	switch config.TargetType {
	case MockTargetType:
		hs, rpc = mockTarget(config, r.httpClient)
	case DomainTargetType:
		hs, rpc = domainTarget(config, r.httpClient)
	case DiscoveryTargetType:
		hs, rpc = discoveryTarget(config, r.httpClient)
	}
	extra := config.Extra
	if extra == nil {
		extra = make(map[string]any)
	}
	trans := &transportImpl{
		rewriteStrategy: rewrite,
		targetSelector:  hs,
		rpc:             rpc,
		config:          config,
	}
	switch config.MatchType {
	case FullMatchType:
		err = fullMatchTransport(r, config, trans)
	case PrefixMatchType:
		err = prefixMatchTransport(r, config, trans)
	case ExprMatchType:
		err = exprMatchTransport(r, config, trans)
	}
	return err
}
