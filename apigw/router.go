package apigw

import (
	"errors"
	"fmt"
	"github.com/LeeZXin/zsf-utils/httputil"
	"github.com/LeeZXin/zsf-utils/trieutil"
	"github.com/LeeZXin/zsf/apigw/hexpr"
	"github.com/LeeZXin/zsf/services/discovery"
	"github.com/gin-gonic/gin"
	"net/http"
)

type LbPolicy string

const (
	RoundRobinPolicy         LbPolicy = "round_robin"
	WeightedRoundRobinPolicy LbPolicy = "weighted_round_robin"
)

// 路由策略

type MatchType string

type MockContentType string

const (
	// FullMatchType 全匹配
	FullMatchType MatchType = "fullMatch"
	// PrefixMatchType 前缀匹配
	PrefixMatchType MatchType = "prefixMatch"
	// ExprMatchType 表达式匹配
	ExprMatchType MatchType = "exprMatch"
	// MockJsonType json格式
	MockJsonType MockContentType = "json"
	// MockStringType string格式
	MockStringType MockContentType = "string"
)

type Target struct {
	Weight int    `json:"weight"`
	Target string `json:"target"`
}

func (t *Target) Validate() error {
	if t.Target == "" {
		return errors.New("empty target")
	}
	return nil
}

type MockContent struct {
	Header      map[string]string `json:"header"`
	ContentType MockContentType   `json:"contentType"`
	StatusCode  int               `json:"statusCode"`
	RespStr     string            `json:"respStr"`
}

func (m *MockContent) Validate() error {
	if m.StatusCode > http.StatusNetworkAuthenticationRequired || m.StatusCode < http.StatusContinue {
		return fmt.Errorf("unsupported mock status code: %v", m.StatusCode)
	}
	switch m.ContentType {
	case MockJsonType, MockStringType:
		return nil
	default:
		return fmt.Errorf("unsupported mock content type: %v", m.ContentType)
	}
}

// RouterConfig 路由配置信息
type RouterConfig struct {
	Id string `json:"id" yaml:"id"`
	// MatchType 匹配模式
	MatchType MatchType `json:"matchType" yaml:"matchType"`
	// Path url path
	Path string `json:"path" yaml:"path"`
	// Expr 表达式
	Expr *hexpr.PlainInfo `json:"expr" yaml:"expr"`
	// ServiceName 服务名称 用于服务发现
	ServiceName string `json:"serviceName" yaml:"serviceName"`
	// Targets 转发目标 配置权重信息
	Targets []Target `json:"targets" yaml:"targets"`
	// TargetType 转发目标类型 服务发现或ip域名转发
	TargetType TargetType `json:"targetType" yaml:"targetType"`
	// TargetLbPolicy 负载均衡策略
	TargetLbPolicy LbPolicy `json:"targetLbPolicy" yaml:"targetLbPolicy"`
	// RewriteType 路径重写类型
	RewriteType RewriteType `json:"rewriteType" yaml:"rewriteType"`
	// ReplacePath 路径完全覆盖path
	ReplacePath string `json:"replacePath" yaml:"replacePath"`
	// MockContent mock数据
	MockContent *MockContent `json:"mockContent" yaml:"mockContent"`
	// AuthConfig 鉴权配置
	AuthConfig *AuthConfig `json:"authConfig" yaml:"authConfig"`
	// 是否需要鉴权
	NeedAuth bool `json:"needAuth" yaml:"needAuth"`
	// 自定义鉴权函数
	AuthFunc AuthFunc `json:"-" yaml:"-"`
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
		if r.Expr == nil {
			return errors.New("empty expression")
		}
		err := r.Expr.Validate()
		if err != nil {
			return err
		}
	} else {
		if r.Path == "" {
			return errors.New("empty path")
		}
	}
	switch r.TargetType {
	case DiscoveryTargetType, DomainTargetType:
	case MockTargetType:
		if r.MockContent == nil {
			return errors.New("empty mock content")
		}
		err := r.MockContent.Validate()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported target type: %v", r.TargetType)
	}
	if r.TargetType == DomainTargetType {
		if len(r.Targets) == 0 {
			return errors.New("empty target")
		}
		for _, target := range r.Targets {
			if err := target.Validate(); err != nil {
				return err
			}
		}
	}
	if r.TargetType != MockTargetType {
		switch r.TargetLbPolicy {
		case RoundRobinPolicy, WeightedRoundRobinPolicy:
		default:
			return fmt.Errorf("unsupported lb policy: %v", r.TargetLbPolicy)
		}
		switch r.RewriteType {
		case CopyFullPathRewriteType, StripPrefixRewriteType:
		case ReplaceAnyRewriteType:
			if r.ReplacePath == "" {
				return errors.New("empty replace path")
			}
		default:
			return fmt.Errorf("unsupported rewriteType: %v", r.RewriteType)
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
