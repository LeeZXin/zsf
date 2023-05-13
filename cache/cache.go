package cache

// 三种通用缓存实现
// single_item_cache, 单数据缓存信息，带有过期时间
// map_cache, 多数据缓存，没有过期时间
// expire_map_cache, 多数据缓存，带有过期时间
// 主要用于httpclient等服务信息缓存实现

type SupplierWithKey func(string) (any, error)

type Supplier func() (any, error)
