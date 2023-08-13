package strutil

import (
	"fmt"
	"testing"
)

func TestConcat(t *testing.T) {
	fmt.Println(Concat([]any{
		"11",
		"22",
		33,
		44,
		55,
	}, ","))
}
