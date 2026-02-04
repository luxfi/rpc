// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package rpc provides protocol-agnostic RPC abstractions for the Lux ecosystem.
//
// # Transport Selection
//
// ZAP is the default transport, optimized for high-frequency trading and VM communication.
// Use build tags to enable alternative transports:
//
//	go build              # ZAP only (default, fastest)
//	go build -tags grpc   # Enable gRPC transport
//	go build -tags json   # Enable JSON-RPC transport
//
// # Usage
//
// Client usage:
//
//	client, err := rpc.Dial(ctx, "localhost:9000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Structured call (auto JSON encoding)
//	var result MyResponse
//	err = client.Call(ctx, "Service.Method", &MyRequest{...}, &result)
//
//	// Raw call (zero-copy, for HFT)
//	resp, err := client.CallRaw(ctx, "trade", orderBytes)
//
// Server usage:
//
//	server, err := rpc.Listen(":9000")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register raw handler for low-latency
//	server.RegisterRaw("trade", func(ctx context.Context, payload []byte) ([]byte, error) {
//	    // Process order with zero-copy
//	    return processOrder(payload)
//	})
//
//	server.Serve(ctx)
//
// # Performance
//
// ZAP transport benchmarks (Apple M1 Max):
//
//	BenchmarkRoundTrip/1KB    46ns encode, 19ns decode, 0 allocs
//	BenchmarkRoundTrip/100KB  1.6μs encode, 19ns decode, 0 allocs
//	BenchmarkRoundTrip/1MB    20μs encode, 19ns decode, 0 allocs
//
// ZAP vs Protobuf comparison:
//
//	1KB:   7.9x faster encode, 19.8x faster decode
//	100KB: 8.5x faster encode, 658x faster decode
//	1MB:   7x faster encode, 2866x faster decode
//
// # Architecture
//
// The package separates concerns:
//
//   - client.go: Protocol-agnostic Client and Server interfaces
//   - codec.go: Codec interface for message encoding
//   - transport.go: Transport registry for build-tag extensibility
//   - dial.go: Dial and Listen factory functions
//   - zap.go: ZAP transport implementation (default)
//   - dial_grpc.go: gRPC transport (requires -tags grpc)
//
// Application code should only depend on the Client/Server interfaces,
// making transport selection a deployment decision rather than a code change.
package rpc
