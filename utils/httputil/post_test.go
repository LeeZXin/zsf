package httputil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostNilResp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	if err := Post(context.Background(), nil, srv.URL, map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPostStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("no"))
	}))
	defer srv.Close()
	if err := Post(context.Background(), nil, srv.URL, map[string]int{"a": 1}, nil); err == nil {
		t.Fatal("非 200 应返回 error")
	}
}

func TestPostUnmarshal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"n":7}`))
	}))
	defer srv.Close()
	var resp struct {
		N int `json:"n"`
	}
	if err := Post(context.Background(), nil, srv.URL, map[string]int{"a": 1}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.N != 7 {
		t.Fatalf("n=%d", resp.N)
	}
}
