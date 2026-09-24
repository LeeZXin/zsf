package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsContextOverflow(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{context.Canceled, false},
		{context.DeadlineExceeded, false},
		{errors.New("rate limit: too many tokens"), false},
		{errors.New("too many requests"), false},
		{errors.New("maximum context length exceeded"), true},
		{errors.New("HTTP 400: prompt is too long"), true},
		{errors.New("This model's maximum prompt length is 128000"), true},
		{errors.New("Please reduce the length of the messages"), true},
		{errors.New("context_length_exceeded"), true},
		{errors.New("network timeout"), false},
	}
	for _, tc := range cases {
		if got := IsContextOverflow(tc.err); got != tc.want {
			t.Errorf("IsContextOverflow(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsContextOverflowWrapped(t *testing.T) {
	inner := errors.New("This model's maximum context length is 64000 tokens")
	err := errors.New("failed to create chat stream completion: " + inner.Error())
	if !IsContextOverflow(err) {
		t.Fatal("wrapped overflow should still match")
	}
	if !strings.Contains(err.Error(), "maximum context length") {
		t.Fatal("sanity")
	}
}
