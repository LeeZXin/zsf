package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

type refreshMapCacheResult struct {
	err               error
	data              any
	shouldRefreshTime atomic.Value
	shouldExpireTime  time.Time
}

type RefreshMapCache struct {
	SupplierWithKey SupplierWithKey
	RefreshDuration time.Duration
	ExpireDuration  time.Duration
	mapCache        sync.Map
}

type supplierWithRefreshMapCacheResult func() *refreshMapCacheResult

// Get 读取缓存
func (c *RefreshMapCache) Get(key string) (any, error) {
	now := time.Now()
	value, ok := c.mapCache.Load(key)
	if ok {
		result := value.(supplierWithRefreshMapCacheResult)
		val := result()
		if !val.shouldExpireTime.IsZero() && val.shouldExpireTime.Before(now) {
			c.mapCache.Delete(key)
		} else if c.RefreshDuration <= 0 {
			return val.data, val.err
		} else {
			shouldRefreshTime := val.shouldRefreshTime.Load().(time.Time)
			//如果过期 并发获取旧缓存 只有一个goroutine才会去获取新值
			if c.RefreshDuration <= 0 || shouldRefreshTime.After(now) {
				return val.data, val.err
			}
			next := now.Add(c.RefreshDuration)
			if !val.shouldRefreshTime.CompareAndSwap(shouldRefreshTime, next) {
				return val.data, val.err
			}
			data, err := c.SupplierWithKey(key)
			if err != nil {
				//获取失败 重置过期时间
				val.shouldRefreshTime.Store(shouldRefreshTime)
				//返回旧值
				return val.data, val.err
			}
			if c.ExpireDuration > 0 {
				val.shouldExpireTime = time.Now().Add(c.ExpireDuration)
			}
			val.data = data
			val.err = err
			return data, err
		}
	}
	result := c.blockGet(key)()
	if result.err != nil {
		c.mapCache.Delete(key)
	}
	return result.data, result.err
}

// blockGet 阻塞式获取缓存，一个key在多goroutine情况下被加载一次
func (c *RefreshMapCache) blockGet(key string) supplierWithRefreshMapCacheResult {
	var (
		wg sync.WaitGroup
		f  supplierWithRefreshMapCacheResult
	)
	wg.Add(1)
	actual, loaded := c.mapCache.LoadOrStore(key, supplierWithRefreshMapCacheResult(func() *refreshMapCacheResult {
		wg.Wait()
		return f()
	}))
	// 加载了老数据
	if loaded {
		return actual.(supplierWithRefreshMapCacheResult)
	}
	data, err := c.SupplierWithKey(key)
	ep := time.Now().Add(c.RefreshDuration)
	shouldRefreshTime := atomic.Value{}
	shouldRefreshTime.Store(ep)
	mr := &refreshMapCacheResult{
		err:               err,
		data:              data,
		shouldRefreshTime: shouldRefreshTime,
	}
	f = func() *refreshMapCacheResult {
		return mr
	}
	c.mapCache.Store(key, f)
	wg.Done()
	return f
}

// AllKeys 获取所有的key
func (c *RefreshMapCache) AllKeys() []string {
	result := make([]string, 0, 8)
	c.mapCache.Range(func(key, value any) bool {
		result = append(result, key.(string))
		return true
	})
	return result
}
