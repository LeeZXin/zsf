package hexpr

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"hash/crc32"
	"strconv"
	"sync"
)

// 提取数据
const (
	// HeaderSource 从header提取数据
	HeaderSource = "header"
	// CookieSource 从Cookie提取数据
	CookieSource = "cookie"
	// PathSource 从url path提取数据
	PathSource = "path"
	// HostSource 从host提取数据
	HostSource = "host"
	// CrcMod2HeaderSource header中某个数据hash获取数据
	CrcMod2HeaderSource = "crcMod2Header"
)

type Fetcher func(*gin.Context, string) string

var (
	fetcherMap = sync.Map{}
)

func init() {
	m := map[string]Fetcher{
		HeaderSource: func(ctx *gin.Context, key string) string {
			return ctx.GetHeader(key)
		},
		CookieSource: func(ctx *gin.Context, key string) string {
			cookie, err := ctx.Cookie(key)
			if err != nil {
				return ""
			}
			return cookie
		},
		HostSource: func(ctx *gin.Context, key string) string {
			return ctx.GetHeader("origin")
		},
		PathSource: func(ctx *gin.Context, key string) string {
			return ctx.Request.URL.Path
		},
		CrcMod2HeaderSource: func(ctx *gin.Context, key string) string {
			return crcMod(ctx.GetHeader(key), 2)
		},
	}
	// hash取模到100
	for i := 2; i < 100; i++ {
		d := fmt.Sprintf("crcMod%dHeader", i)
		k := i
		m[d] = func(ctx *gin.Context, key string) string {
			return crcMod(ctx.GetHeader(key), k)
		}
	}
	for k, fetcher := range m {
		fetcherMap.Store(k, fetcher)
	}
}

func crcMod(val string, mod int) string {
	if val == "" {
		return "unknown"
	}
	i := crc32.ChecksumIEEE([]byte(val)) % uint32(mod)
	return strconv.Itoa(int(i))
}

func RegisterFetcher(source string, fetcher Fetcher) {
	if source != "" && fetcher != nil {
		fetcherMap.Store(source, fetcher)
	}
}
