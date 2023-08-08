package threadutil

import (
	"fmt"
	"testing"
)

func TestRunSafe(t *testing.T) {
	var err error = nil
	fmt.Println(RunSafe(func() {
		fmt.Println(err.Error())
	}, func() {
		fmt.Println("clean up")
	}))

}
