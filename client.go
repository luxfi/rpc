// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package rpc provides protocol-agnostic RPC client/server abstractions.
// Applications use these interfaces without caring about the underlying
// transport (ZAP, gRPC, JSON-RPC, etc.).
//
// ZAP is the default transport. Use build tags to enable alternatives:
//
//	go build -tags grpc  # Enable gRPC transport
//	go build -tags json  # Enable JSON-RPC transport
package rpc

import (
	"context"
	"io"
)

// Client is the protocol-agnostic RPC client interface.
// All application code should use this interface.
type Client interface {
	// Call makes a synchronous RPC call
	Call(ctx context.Context, method string, args, reply interface{}) error

	// CallRaw makes a call with raw bytes (for zero-copy scenarios)
	CallRaw(ctx context.Context, method string, payload []byte) ([]byte, error)

	// Notify sends a one-way message (no response expected)
	Notify(ctx context.Context, method string, args interface{}) error

	// Close closes the connection
	Close() error
}

// Server is the protocol-agnostic RPC server interface.
type Server interface {
	// Register registers a service handler
	Register(name string, handler interface{}) error

	// RegisterRaw registers a raw byte handler
	RegisterRaw(method string, handler RawHandler) error

	// Serve starts serving requests (blocks until context cancelled)
	Serve(ctx context.Context) error

	// Close stops the server
	Close() error

	// Addr returns the server's listen address
	Addr() string
}

// RawHandler handles raw byte RPC calls (for zero-copy)
type RawHandler func(ctx context.Context, payload []byte) ([]byte, error)

// Codec encodes/decodes RPC messages
type Codec interface {
	Encode(v interface{}) ([]byte, error)
	Decode(data []byte, v interface{}) error
}

// Transport represents the underlying transport mechanism
type Transport interface {
	io.Closer
	Send(ctx context.Context, data []byte) error
	Recv(ctx context.Context) ([]byte, error)
}

// DialOption configures client connections
type DialOption func(*dialOptions)

type dialOptions struct {
	codec     Codec
	transport string // "zap", "grpc", "json"
}

// WithCodec sets a custom codec
func WithCodec(c Codec) DialOption {
	return func(o *dialOptions) { o.codec = c }
}

// WithTransport explicitly sets the transport type
func WithTransport(t string) DialOption {
	return func(o *dialOptions) { o.transport = t }
}

// ServerOption configures servers
type ServerOption func(*serverOptions)

type serverOptions struct {
	codec     Codec
	transport string
}

// WithServerCodec sets a custom codec for the server
func WithServerCodec(c Codec) ServerOption {
	return func(o *serverOptions) { o.codec = c }
}

// WithServerTransport explicitly sets the transport type for the server
func WithServerTransport(t string) ServerOption {
	return func(o *serverOptions) { o.transport = t }
}
