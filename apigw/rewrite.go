package apigw

import (
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

type RewriteStrategy interface {
	Rewrite(string) string
}

type NewRewriteStrategyFunc func(RouterConfig) RewriteStrategy

type CopyFullPathRewriteStrategy struct{}

func (*CopyFullPathRewriteStrategy) Rewrite(path string) string {
	return path
}

type StripPrefixRewriteStrategy struct {
	prefix string
}

func (s *StripPrefixRewriteStrategy) Rewrite(path string) string {
	return strings.TrimPrefix(path, s.prefix)
}

type ReplaceAnyRewriteStrategy struct {
	anyPath string
}

func (s *ReplaceAnyRewriteStrategy) Rewrite(path string) string {
	return s.anyPath
}

func copyFullPathStrategy(_ RouterConfig) RewriteStrategy {
	return &CopyFullPathRewriteStrategy{}
}

func stripPrefixStrategy(config RouterConfig) RewriteStrategy {
	return &StripPrefixRewriteStrategy{prefix: config.Path}
}

func replaceAnyStrategy(config RouterConfig) RewriteStrategy {
	return &ReplaceAnyRewriteStrategy{anyPath: config.ReplacePath}
}
