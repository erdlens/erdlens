package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResolveViewPathValidation(t *testing.T) {
	ctx := context.Background()
	timeout := time.Second

	_, _, err := resolveViewPath(ctx, "", "", "", nil, nil, nil, timeout)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required error, got %v", err)
	}

	_, _, err = resolveViewPath(ctx, "schema.erd", "postgres://x", "", nil, nil, nil, timeout)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
