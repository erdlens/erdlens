package main

import (
	"context"
	"fmt"
	"time"

	"github.com/erdlens/erdlens/internal/introspect"
	"github.com/erdlens/erdlens/internal/schema"
)

func introspectDSN(
	ctx context.Context,
	dsn string,
	opts introspect.Options,
	timeout time.Duration,
) (*schema.Schema, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ins, err := introspect.Open(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = ins.Close() }()

	s, err := ins.Introspect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("introspect: %w", err)
	}
	return s, nil
}
