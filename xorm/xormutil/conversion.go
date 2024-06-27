package xormutil

import (
	"encoding/json"
)

type Conversion[T any] struct {
	Data T `json:"data" yaml:"data"`
}

func (c *Conversion[T]) FromDB(content []byte) error {
	return json.Unmarshal(content, &c.Data)
}

func (c *Conversion[T]) ToDB() ([]byte, error) {
	return json.Marshal(c.Data)
}
