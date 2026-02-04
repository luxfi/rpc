// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"testing"
	"time"
)

func TestZAPRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create server
	server, err := Listen(":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	// Register echo handler
	server.RegisterRaw("echo", func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	})

	// Start server in background
	go server.Serve(ctx)

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Connect client
	client, err := Dial(ctx, server.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// Test raw call
	payload := []byte("hello world")
	resp, err := client.CallRaw(ctx, "echo", payload)
	if err != nil {
		t.Fatalf("CallRaw: %v", err)
	}

	if string(resp) != string(payload) {
		t.Errorf("got %q, want %q", resp, payload)
	}
}

func TestZAPCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server, err := Listen(":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	// Register JSON handler
	server.RegisterRaw("add", func(ctx context.Context, payload []byte) ([]byte, error) {
		var req struct{ A, B int }
		if err := defaultCodec.Decode(payload, &req); err != nil {
			return nil, err
		}
		resp := struct{ Sum int }{Sum: req.A + req.B}
		return defaultCodec.Encode(resp)
	})

	go server.Serve(ctx)
	time.Sleep(10 * time.Millisecond)

	client, err := Dial(ctx, server.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	var resp struct{ Sum int }
	err = client.Call(ctx, "add", struct{ A, B int }{A: 2, B: 3}, &resp)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if resp.Sum != 5 {
		t.Errorf("got %d, want 5", resp.Sum)
	}
}

func BenchmarkZAPRoundTrip(b *testing.B) {
	ctx := context.Background()

	server, err := Listen(":0")
	if err != nil {
		b.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	server.RegisterRaw("echo", func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	})

	go server.Serve(ctx)
	time.Sleep(10 * time.Millisecond)

	client, err := Dial(ctx, server.Addr())
	if err != nil {
		b.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	payload := make([]byte, 1024)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := client.CallRaw(ctx, "echo", payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}
