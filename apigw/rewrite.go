package apigw

import (
	"github.com/LeeZXin/zsf/common"
	"net/http"
	"strings"
)

// 路径重写策略
const (
	// CopyFullPathRewriteType 不重写
	CopyFullPathRewriteType = "copyFullPath"
	// StripPrefixRewriteType 去除前缀
	StripPrefixRewriteType = "stripPrefix"
	// ReplaceAnyRewriteType 完全重写
	ReplaceAnyRewriteType = "replaceAny"
)

var (
	rewriteStrategyFuncMap = map[string]NewRewriteStrategyFunc{
		CopyFullPathRewriteType: func(config RouterConfig) RewriteStrategy {
			return &CopyFullPathRewriteStrategy{}
		},
		StripPrefixRewriteType: func(config RouterConfig) RewriteStrategy {
			return &StripPrefixRewriteStrategy{prefix: config.Path}
		},
		ReplaceAnyRewriteType: func(config RouterConfig) RewriteStrategy {
			return &ReplaceAnyRewriteStrategy{anyPath: config.ReplacePath}
		},
	}
)

type RewriteStrategy interface {
	Rewrite(*string, http.Header)
}

type NewRewriteStrategyFunc func(RouterConfig) RewriteStrategy

type CopyFullPathRewriteStrategy struct{}

func (*CopyFullPathRewriteStrategy) Rewrite(path *string, header http.Header) {
	labelHeader(header)
}

type StripPrefixRewriteStrategy struct {
	prefix string
}

func (s *StripPrefixRewriteStrategy) Rewrite(path *string, header http.Header) {
	p := strings.TrimPrefix(*path, s.prefix)
	labelHeader(header)
	*path = p
}

type ReplaceAnyRewriteStrategy struct {
	anyPath string
}

func (s *ReplaceAnyRewriteStrategy) Rewrite(path *string, header http.Header) {
	*path = s.anyPath
	labelHeader(header)
}

func labelHeader(header http.Header) {
	header.Set("z-gw-type", "zgw")
	header.Add("x-forwarded-for", common.GetLocalIp())
}
