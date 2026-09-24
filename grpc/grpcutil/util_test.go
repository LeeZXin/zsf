package grpcutil

import (
	"testing"

	"github.com/LeeZXin/zsf/grpc/server/testproto"
)

func TestPackMethodName(t *testing.T) {
	if got := PackMethodName("pkg.Svc.Method"); got != "/pkg.Svc/Method" {
		t.Fatalf("got %q", got)
	}
	if got := PackMethodName("Method"); got != "Method" {
		t.Fatalf("不含 . 时应原样, got %q", got)
	}
}

func TestMessageJSONRoundTrip(t *testing.T) {
	in := &testproto.HelloReq{Name: "world"}
	s, err := MessageToString(in)
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("空 JSON")
	}
	var obj map[string]any
	if err := MessageToObject(in, &obj); err != nil {
		t.Fatal(err)
	}
	out := new(testproto.HelloReq)
	if err := ObjectToMessage(obj, out); err != nil {
		t.Fatal(err)
	}
	if out.GetName() != "world" {
		t.Fatalf("name=%q", out.GetName())
	}
	out2 := new(testproto.HelloReq)
	if err := ObjectToMessage(map[string]any{"name": "x", "unknown": 1}, out2); err != nil {
		t.Fatal(err)
	}
	if out2.GetName() != "x" {
		t.Fatalf("unknown 字段应忽略, name=%q", out2.GetName())
	}
}
