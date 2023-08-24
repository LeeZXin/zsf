package main

import (
	"context"
	"fmt"
	"github.com/LeeZXin/zsf/xorm/mysqlstore"
	"github.com/LeeZXin/zsf/zsf"
)

func main() {
	ctx, closer := mysqlstore.Context(context.Background())
	defer closer.Close()
	session := mysqlstore.GetXormSession(ctx)
	var ret []int64
	err := session.Table("xxx").Cols("id").Find(&ret)
	fmt.Println(ret, err)
	zsf.Run()
}
