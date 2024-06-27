package xormutil

import (
	"fmt"
	"testing"
)

type Person struct {
	Name string `json:"name"`
}

func TestConversion(t *testing.T) {
	var m = new(Conversion[*Person])
	fmt.Println(m.FromDB([]byte(`{"name": "b"}`)))
	fmt.Println(m.Data)
}
