package client

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestTimeoutUnaryInterceptorDoesNotCapExistingDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	err := timeoutUnaryInterceptor(parent, "/m", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			d, ok := ctx.Deadline()
			if !ok {
				t.Fatal("应保留 deadline")
			}
			if time.Until(d) < time.Hour {
				t.Fatalf("更长的父 deadline 不应被砍到 60s，剩余 %v", time.Until(d))
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutUnaryInterceptorAddsDefaultWhenMissing(t *testing.T) {
	err := timeoutUnaryInterceptor(context.Background(), "/m", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			d, ok := ctx.Deadline()
			if !ok {
				t.Fatal("无 deadline 时应补默认超时")
			}
			remain := time.Until(d)
			if remain > 60*time.Second || remain < 50*time.Second {
				t.Fatalf("默认超时应约 60s，剩余 %v", remain)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}
