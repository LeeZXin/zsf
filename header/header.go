package header

import (
	"context"
	"github.com/LeeZXin/zsf/rpc"
)

const (
	UserId   = "z-user-id"
	UserName = "z-user-name"
	AppId    = "z-app-id"
)

func GetUserId(ctx context.Context) string {
	return rpc.GetHeaders(ctx).Get(UserId)
}

func GetUserName(ctx context.Context) string {
	return rpc.GetHeaders(ctx).Get(UserName)
}

func GetAppId(ctx context.Context) string {
	return rpc.GetHeaders(ctx).Get(AppId)
}
