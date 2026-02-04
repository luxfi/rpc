// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"fmt"
	"net"
)

// Dial connects to an RPC server using the default transport (ZAP).
// Use DialWithOptions for transport selection.
func Dial(ctx context.Context, addr string, opts ...DialOption) (Client, error) {
	o := &dialOptions{
		transport: "zap", // ZAP is default
	}
	for _, opt := range opts {
		opt(o)
	}

	switch o.transport {
	case "zap":
		return dialZAP(ctx, addr, o)
	default:
		return nil, fmt.Errorf("unknown transport: %s", o.transport)
	}
}

// Listen creates an RPC server listener using the default transport (ZAP).
func Listen(addr string, opts ...ServerOption) (Server, error) {
	o := &serverOptions{
		transport: "zap", // ZAP is default
	}
	for _, opt := range opts {
		opt(o)
	}

	switch o.transport {
	case "zap":
		return listenZAP(addr, o)
	default:
		return nil, fmt.Errorf("unknown transport: %s", o.transport)
	}
}

// dialZAP creates a ZAP client
func dialZAP(ctx context.Context, addr string, o *dialOptions) (Client, error) {
	conn, err := ZAPDial(ctx, addr)
	if err != nil {
		return nil, err
	}
	return &zapClient{
		conn:  conn,
		codec: o.codec,
	}, nil
}

// listenZAP creates a ZAP server
func listenZAP(addr string, o *serverOptions) (Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &zapServer{
		listener: listener,
		handlers: make(map[string]RawHandler),
		codec:    o.codec,
	}, nil
}

// zapClient implements Client using ZAP transport
type zapClient struct {
	conn  *ZAPConn
	codec Codec
}

func (c *zapClient) Call(ctx context.Context, method string, args, reply interface{}) error {
	var payload []byte
	var err error

	if args != nil {
		if c.codec != nil {
			payload, err = c.codec.Encode(args)
		} else {
			payload, err = defaultCodec.Encode(args)
		}
		if err != nil {
			return fmt.Errorf("encode args: %w", err)
		}
	}

	resp, err := c.conn.Call(ctx, method, payload)
	if err != nil {
		return err
	}

	if reply != nil && len(resp) > 0 {
		if c.codec != nil {
			err = c.codec.Decode(resp, reply)
		} else {
			err = defaultCodec.Decode(resp, reply)
		}
		if err != nil {
			return fmt.Errorf("decode reply: %w", err)
		}
	}
	return nil
}

func (c *zapClient) CallRaw(ctx context.Context, method string, payload []byte) ([]byte, error) {
	return c.conn.Call(ctx, method, payload)
}

func (c *zapClient) Notify(ctx context.Context, method string, args interface{}) error {
	var payload []byte
	var err error

	if args != nil {
		if c.codec != nil {
			payload, err = c.codec.Encode(args)
		} else {
			payload, err = defaultCodec.Encode(args)
		}
		if err != nil {
			return fmt.Errorf("encode args: %w", err)
		}
	}

	return c.conn.Notify(ctx, method, payload)
}

func (c *zapClient) Close() error {
	return c.conn.Close()
}

// zapServer implements Server using ZAP transport
type zapServer struct {
	listener net.Listener
	handlers map[string]RawHandler
	server   *ZAPServer
	codec    Codec
}

func (s *zapServer) Register(name string, handler interface{}) error {
	// TODO: Use reflection to register method handlers
	return fmt.Errorf("Register not yet implemented - use RegisterRaw")
}

func (s *zapServer) RegisterRaw(method string, handler RawHandler) error {
	s.handlers[method] = handler
	return nil
}

func (s *zapServer) Serve(ctx context.Context) error {
	s.server = NewZAPServer(s.listener, ZAPHandlerFunc(func(ctx context.Context, method string, payload []byte) ([]byte, error) {
		handler, ok := s.handlers[method]
		if !ok {
			return nil, fmt.Errorf("unknown method: %s", method)
		}
		return handler(ctx, payload)
	}))
	return s.server.Serve(ctx)
}

func (s *zapServer) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	return s.listener.Close()
}

func (s *zapServer) Addr() string {
	return s.listener.Addr().String()
}
