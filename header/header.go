package header

import (
	"context"
)

const (
	UserId   = "z-user-id"
	UserName = "z-user-name"
	AppId    = "z-app-id"
)

func GetUserId(ctx context.Context) string {
	return GetHeaders(ctx).Get(UserId)
}

func GetUserName(ctx context.Context) string {
	return GetHeaders(ctx).Get(UserName)
}

func GetAppId(ctx context.Context) string {
	return GetHeaders(ctx).Get(AppId)
}
