/*
Package cacheutil 提供内存缓存与线程安全值容器：
  - ExpirableCache: 带过期时间的泛型缓存，集成 singleflight 防击穿与负缓存（expirable_cache.go）
  - Locked: 由 sync.RWMutex 保护的泛型值容器（locked.go）

两者均为并发安全实现。

选型（本仓库现状）：corelet 的缓存一律是 Locked + 幂等全量重建（见 corelet/base/cache、
internal/token-hub/internal/core/cache），刷新由 inmq 变更消息触发，没有按 key 过期的需求；
ExpirableCache 至今没有调用方，属框架预留能力——确实需要"按 key 过期 + 回源 + 防击穿"
时才引入，不要用它替换既有的全量重建模式（后者是幂等的，丢了刷新消息也只是延迟收敛）。
*/
package cacheutil

import (
	"context"
	"log"
	"time"

	"github.com/LeeZXin/zsf/logger"

	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

/*
Getter 是数据获取函数类型，用于从数据源（如数据库）加载缓存值。
泛型参数 T 为业务数据类型，缓存实际存储的是 *T（指针），以支持 nil 表示"不存在"。

入参：
  - ctx: 调用方传入的 context，用于超时控制和日志追踪
  - key: 缓存键，由调用方定义（如广告 ID、请求 ID 等）

返回值：
  - *T: 数据指针，nil 表示数据不存在
  - error: 获取过程中的错误
*/
type Getter[T any] func(context.Context, string) (*T, error)

/*
ExpirableCache 是一个带过期时间的泛型缓存，集成 singleflight 防击穿机制。

核心特性：
 1. 基于 go-cache 实现内存缓存，每个 key 独立设置过期时间
 2. 缓存未命中时自动调用 Getter 回源加载数据
 3. 集成 singleflight，同一 key 的并发回源请求会被合并为一次调用，防止缓存击穿
 4. 对 Getter 返回的 nil 结果（数据不存在）进行负缓存（3 秒），避免频繁回源查询不存在的数据
 5. 对 Getter 返回 error 的情况同样进行负缓存（3 秒），避免故障时对下游造成压力

设计要点：
  - 正缓存（有数据）的过期时间由 NewExpirableCache 的 duration 参数控制
  - 负缓存（无数据/异常）统一使用 3 秒过期时间，既保护下游又允许快速恢复
  - singleflight 函数始终返回 nil error，让等待的 goroutine 共享同一份结果（含 nil 结果）

注意：本仓库（corelet）暂无调用方——现有缓存走 Locked + 全量重建，见包注释的"选型"。
*/
type ExpirableCache[T any] struct {
	c *cache.Cache        // go-cache 实例，线程安全
	s *singleflight.Group // singleflight 组，合并同一 key 的并发回源请求
	g Getter[T]           // 回源数据获取函数
	d time.Duration       // 正缓存过期时间
}

/*
NewExpirableCache 创建一个 ExpirableCache 实例。

参数：
  - duration: 正缓存（Getter 成功返回非 nil 数据）的过期时间，必须 > 0
  - getter: 回源数据获取函数，缓存未命中时调用，不能为 nil

Panic 场景：
  - duration <= 0：过期时间必须为正数
  - getter == nil：回源函数不能为空，否则无法加载数据

实现细节：
  - go-cache 的默认过期时间设为 1 分钟（实际上每个 key 都会通过 Set 显式指定过期时间，此默认值不会被使用）
  - go-cache 的清理间隔设为 10 分钟，过期条目最多残留 10 分钟后被清理（但 Get 时会检查过期并返回 miss，不会返回脏数据）
*/
func NewExpirableCache[T any](duration time.Duration, getter Getter[T]) *ExpirableCache[T] {
	if duration <= 0 {
		log.Fatalln("cache expiration duration must be positive")
	}
	if getter == nil {
		log.Fatalln("cache getter is nil")
	}
	return &ExpirableCache[T]{
		c: cache.New(time.Minute, 10*time.Minute),
		s: new(singleflight.Group),
		g: getter,
		d: duration,
	}
}

/*
Get 从缓存中获取 key 对应的数据，缓存未命中时自动回源加载。

执行流程：
 1. 先查 go-cache 内存缓存，命中且非负缓存则直接返回
 2. 未命中（含缓存过期）则进入 singleflight.Do，合并同一 key 的并发请求
 3. singleflight 内的函数调用 Getter 回源：
    a. 成功且返回非 nil 数据 → 以 duration 过期时间写入正缓存
    b. 成功但返回 nil（数据不存在）→ 以 3 秒过期时间写入负缓存，返回 nil
    c. 回源失败（err != nil）→ 记录错误日志，以 3 秒过期时间写入负缓存，返回 nil
 4. 返回结果给所有等待该 key 的 goroutine

返回值：
  - *T: 数据指针，缓存命中返回缓存值，未命中返回 Getter 的结果；nil 表示数据不存在或回源失败

注意：
  - 调用方无法区分"数据不存在"和"回源失败"，两者都返回 nil。这是有意为之：避免调用方对失败场景做特殊处理，
    负缓存保证了下一次 Get 在 3 秒内不会再触发回源
  - singleflight 内部使用的 ctx 来自最先到达的 goroutine，其他等待者的 ctx 被忽略。
    如果 Getter 内部依赖 ctx 做超时控制或日志追踪，这是可接受的取舍
*/
func (c *ExpirableCache[T]) Get(ctx context.Context, key string) *T {
	cached, ok := c.c.Get(key)
	if ok {
		// 命中缓存：go-cache 已检查过期时间，过期条目不会返回
		if cached == nil {
			return nil // 负缓存命中，数据不存在
		}
		return cached.(*T)
	}
	// singleflight 防止并发请求击穿缓存：
	// 同一 key 的多个并发 Get 调用中，只有第一个会执行回源函数，
	// 其他 goroutine 等待并共享同一份结果
	v, _, _ := c.s.Do(key, func() (any, error) {
		r, err := c.g(ctx, key)
		if err != nil || r == nil {
			if err != nil {
				logger.Ctx(ctx).Err(err).Msg("")
			}
			// 负缓存：回源失败或数据不存在时，缓存 nil 值 3 秒，
			// 避免对下游造成持续压力，同时保证故障恢复后能快速感知
			c.c.Set(key, nil, 3*time.Second)
			return (*T)(nil), nil
		}
		// 正缓存：以构造时指定的 duration 过期时间写入缓存
		c.c.Set(key, r, c.d)
		return r, nil
	})
	if v == nil {
		return nil
	}
	return v.(*T)
}
