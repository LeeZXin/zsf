package cache

import (
	"errors"
	"sync"
	"time"
)

type SingleItemCache struct {
	ExpireDuration time.Duration
	expireTime     time.Time
	Supplier       Supplier
	data           any
	mutex          sync.RWMutex
}

func (c *SingleItemCache) Get() (any, error) {
	if c.Supplier == nil {
		return nil, errors.New("empty supplier")
	}
	c.mutex.RLock()
	if c.data != nil && (c.expireTime.IsZero() || time.Since(c.expireTime) < 0) {
		defer c.mutex.RUnlock()
		return c.data, nil
	}
	c.mutex.RUnlock()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.data != nil && (c.expireTime.IsZero() || time.Since(c.expireTime) < 0) {
		return c.data, nil
	}
	data, err := c.Supplier()
	if err == nil {
		if c.ExpireDuration > 0 {
			c.expireTime = time.Now().Add(c.ExpireDuration)
		}
		c.data = data
	}
	return c.data, err
}
