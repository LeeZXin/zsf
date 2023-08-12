package timeutil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type P struct {
	U JsonTime `json:"u"`
}

func TestJsonTime_MarshalJSON(t *testing.T) {
	marshal, err := json.Marshal(P{
		U: JsonTime(time.Now()),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(marshal))
}

func TestJsonTime_UnmarshalJSON(t *testing.T) {
	j := `{"u":"2023-08-12 14:43:39"}`
	var ret P
	fmt.Println(json.Unmarshal([]byte(j), &ret), ret)
}
