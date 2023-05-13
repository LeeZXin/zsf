package cache

import (
	"sync"
)

// MapCache 永不过期cache
type MapCache struct {
	SupplierWithKey SupplierWithKey
	mapCache        sync.Map
}

type mapCacheResult struct {
	err  error
	data any
}

type supplierWithMapCacheResult func() *mapCacheResult

// Get 读取缓存
func (c *MapCache) Get(key string) (any, error) {
	value, ok := c.mapCache.Load(key)
	if ok {
		result := value.(supplierWithMapCacheResult)()
		return result.data, result.err
	}
	result := c.blockGet(key)()
	if result.err != nil {
		c.mapCache.Delete(key)
	}
	return result.data, result.err
}

// blockGet 阻塞式获取缓存，一个key在多goroutine情况下被加载一次
func (c *MapCache) blockGet(key string) supplierWithMapCacheResult {
	var (
		wg sync.WaitGroup
		f  supplierWithMapCacheResult
	)
	wg.Add(1)
	actual, loaded := c.mapCache.LoadOrStore(key, supplierWithMapCacheResult(func() *mapCacheResult {
		wg.Wait()
		return f()
	}))
	// 加载了老数据
	if loaded {
		return actual.(supplierWithMapCacheResult)
	}
	data, err := c.SupplierWithKey(key)
	mr := &mapCacheResult{
		err:  err,
		data: data,
	}
	f = func() *mapCacheResult {
		return mr
	}
	wg.Done()
	c.mapCache.Store(key, f)
	return f
}

// AllKeys 获取所有的key
func (c *MapCache) AllKeys() []string {
	result := make([]string, 0, 8)
	c.mapCache.Range(func(key, value any) bool {
		result = append(result, key.(string))
		return true
	})
	return result
}
