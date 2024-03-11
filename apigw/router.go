package apigw

import (
	"errors"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/trieutil"
	"github.com/LeeZXin/zsf/apigw/hexpr"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/gin-gonic/gin"
	"net/http"
)

const (
	RoundRobinPolicy         = "round_robin"
	WeightedRoundRobinPolicy = "weighted_round_robin"
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
	AuthConfig any `json:"authConfig"`
	// 是否需要鉴权
	NeedAuth bool
	// 自定义鉴权函数
	AuthFunc AuthFunc
}

func (r *RouterConfig) FillDefaultVal() {
	if r.TargetLbPolicy == "" {
		r.TargetLbPolicy = RoundRobinPolicy
	}
	if r.RewriteType == "" {
		r.RewriteType = CopyFullPathRewriteType
	}
}
func (r *RouterConfig) Validate() error {
	if r.MatchType == "" {
		return errors.New("empty match type")
	}
	switch r.MatchType {
	case FullMatchType, PrefixMatchType, ExprMatchType:
	default:
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
	switch r.TargetType {
	case DiscoveryTargetType, DomainTargetType, MockTargetType:
	default:
		return errors.New("wrong target type")
	}
	if r.TargetType == DomainTargetType {
		if len(r.Targets) == 0 {
			return errors.New("empty target")
		}
	}
	if r.TargetType != MockTargetType {
		switch r.TargetLbPolicy {
		case RoundRobinPolicy, WeightedRoundRobinPolicy:
		default:
			return errors.New("wrong lb policy")
		}
		switch r.RewriteType {
		case CopyFullPathRewriteType, StripPrefixRewriteType, ReplaceAnyRewriteType:
		default:
			return errors.New("wrong RewriteType")
		}
	}
	return nil
}

type Routers interface {
	putFullMatchTransport(string, Transport)
	putPrefixMatchTransport(string, Transport)
	putExprMatchTransport(*hexpr.Expr, Transport)
	FindTransport(*gin.Context) (Transport, bool)
	AddRouter(RouterConfig) error
}

type routersImpl struct {
	//精确匹配
	fullMatch map[string]Transport
	//前缀匹配
	prefixMatch *trieutil.Trie[Transport]
	//表达式匹配
	exprMatch map[*hexpr.Expr]Transport
	//连接池
	httpClient *http.Client
	//服务发现
	discovery discovery.Discovery
}

type routerOpts struct {
	httpClient *http.Client
	discovery  discovery.Discovery
}

type RouterOpt func(*routerOpts)

func WithHttpClient(httpClient *http.Client) RouterOpt {
	return func(o *routerOpts) {
		o.httpClient = httpClient
	}
}

func WithDiscovery(discovery discovery.Discovery) RouterOpt {
	return func(o *routerOpts) {
		o.discovery = discovery
	}
}

func NewRouters(opts ...RouterOpt) Routers {
	o := new(routerOpts)
	for _, opt := range opts {
		opt(o)
	}
	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = httputil.NewRetryableHttpClient()
	}
	return &routersImpl{
		httpClient: httpClient,
	}
}

func (r *routersImpl) putFullMatchTransport(path string, transport Transport) {
	if r.fullMatch == nil {
		r.fullMatch = make(map[string]Transport, 8)
	}
	r.fullMatch[path] = transport
}

func (r *routersImpl) putPrefixMatchTransport(path string, transport Transport) {
	if r.prefixMatch == nil {
		r.prefixMatch = &trieutil.Trie[Transport]{}
	}
	r.prefixMatch.Insert(path, transport)
}

func (r *routersImpl) putExprMatchTransport(expr *hexpr.Expr, transport Transport) {
	if r.exprMatch == nil {
		r.exprMatch = make(map[*hexpr.Expr]Transport, 8)
	}
	r.exprMatch[expr] = transport
}

func (r *routersImpl) FindTransport(c *gin.Context) (Transport, bool) {
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
func (r *routersImpl) AddRouter(config RouterConfig) error {
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
	transport := &transportImpl{
		rewriteStrategy: rewrite,
		targetSelector:  hs,
		rpc:             rpc,
		config:          config,
	}
	switch config.MatchType {
	case FullMatchType:
		err = fullMatchTransport(r, config, transport)
	case PrefixMatchType:
		err = prefixMatchTransport(r, config, transport)
	case ExprMatchType:
		err = exprMatchTransport(r, config, transport)
	}
	return err
}
