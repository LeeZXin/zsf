package server

import (
	"context"
	"io"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRecoverUnaryInterceptorPanic(t *testing.T) {
	interceptor := recoverUnaryInterceptor()
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"},
		func(context.Context, any) (any, error) {
			panic("boom")
		})
	if err == nil {
		t.Fatal("panic 应转为 error，而不是再次 panic 或返回 nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("期望 codes.Internal，got %v", err)
	}
}

type stubStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stubStream) Context() context.Context { return s.ctx }
func (s *stubStream) RecvMsg(any) error        { return io.EOF }
func (s *stubStream) SendMsg(any) error        { return nil }
func (s *stubStream) SetHeader(metadata.MD) error {
	return nil
}
func (s *stubStream) SendHeader(metadata.MD) error { return nil }
func (s *stubStream) SetTrailer(metadata.MD)       {}

func TestRecoverStreamInterceptorPanic(t *testing.T) {
	interceptor := recoverStreamInterceptor()
	err := interceptor(nil, &stubStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Watch"},
		func(any, grpc.ServerStream) error {
			panic("boom")
		})
	if err == nil {
		t.Fatal("stream panic 应转为 error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("期望 codes.Internal，got %v", err)
	}
}
