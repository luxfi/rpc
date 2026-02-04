//go:build grpc

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	// Register gRPC transport when build tag is enabled
	registerTransport("grpc", dialGRPC, listenGRPC)
}

func dialGRPC(ctx context.Context, addr string, o *dialOptions) (Client, error) {
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	return &grpcClient{conn: conn, codec: o.codec}, nil
}

func listenGRPC(addr string, o *serverOptions) (Server, error) {
	return nil, fmt.Errorf("grpc server not yet implemented")
}

type grpcClient struct {
	conn  *grpc.ClientConn
	codec Codec
}

func (c *grpcClient) Call(ctx context.Context, method string, args, reply interface{}) error {
	return c.conn.Invoke(ctx, method, args, reply)
}

func (c *grpcClient) CallRaw(ctx context.Context, method string, payload []byte) ([]byte, error) {
	var resp []byte
	err := c.conn.Invoke(ctx, method, payload, &resp)
	return resp, err
}

func (c *grpcClient) Notify(ctx context.Context, method string, args interface{}) error {
	return c.conn.Invoke(ctx, method, args, nil)
}

func (c *grpcClient) Close() error {
	return c.conn.Close()
}
