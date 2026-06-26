// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestZAPRegisterRawDuringServe exercises the RWMutex that guards
// zapServer.handlers: RegisterRaw (writer, s.mu.Lock) must be safe concurrent
// with the Serve dispatch read (reader, s.mu.RLock) so a host can register
// native ZAP methods dynamically while the accept loop is already running.
//
// Topology: one server with the accept loop already serving; many writer
// goroutines registering methods mid-flight; one shared client driven by many
// reader goroutines streaming CallRaw. Because the server spawns a goroutine
// per request, the dispatch read runs concurrently across many goroutines AND
// concurrently with every RegisterRaw write — the exact interleaving the lock
// protects. Before the fix this was an unsynchronized map read/write that the
// race detector trips; run with -race.
//
// Two properties are asserted:
//  1. No data race / no corruption while registration overlaps dispatch (-race).
//  2. Every dynamically registered method becomes callable on the live server
//     and dispatches to its OWN handler — the response carries the method name,
//     proving registration is visible and handlers are not cross-wired.
//
// Deterministic: no time or randomness drives control flow. Method selection is
// modular, registration completion is observed via a WaitGroup (which also
// establishes the happens-before for the final gate), and readiness needs no
// sleep because the listener backlog accepts the dial before Serve calls Accept.
func TestZAPRegisterRawDuringServe(t *testing.T) {
	const (
		methods    = 50 // distinct methods, one writer goroutine each
		reRegister = 40 // re-registrations per writer: keeps the write side busy
		readers    = 20 // concurrent reader goroutines sharing one client
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Accept loop runs first; methods are registered later, mid-flight.
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ctx) }()

	// Drain the accept loop on the way out (LIFO: Close runs first to stop
	// Serve, then this read confirms Serve returned cleanly).
	defer func() {
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("Serve returned error: %v", err)
			}
		case <-ctx.Done():
			t.Errorf("Serve did not return after Close: %v", ctx.Err())
		}
	}()
	defer server.Close()

	// The listener backlog accepts this dial immediately, before Serve reaches
	// Accept, so no readiness sleep is needed.
	client, err := Dial(ctx, server.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	methodName := func(i int) string { return "m" + strconv.Itoa(i) }
	// Each handler echoes "<method>:<payload>" so a correct response proves
	// CallRaw(method) reached handler(method) and the payload survived intact.
	want := func(method string, payload []byte) string { return method + ":" + string(payload) }
	payload := []byte("p")

	var (
		start      = make(chan struct{}) // released once to fire every goroutine together
		done       = make(chan struct{}) // closed after every writer finishes
		registrars sync.WaitGroup
		readerWG   sync.WaitGroup
		okCalls    atomic.Int64 // successful dispatches observed during the race window
	)

	// Writers: each goroutine registers its own method repeatedly, concurrent
	// with the running accept loop and the reader storm. This is the write side
	// of the race (s.mu.Lock). Re-registration is idempotent (same handler
	// value) and keeps writers contending on the map for the whole window.
	registrars.Add(methods)
	for i := 0; i < methods; i++ {
		i := i
		go func() {
			defer registrars.Done()
			method := methodName(i)
			handler := func(ctx context.Context, p []byte) ([]byte, error) {
				return []byte(want(method, p)), nil
			}
			<-start
			for r := 0; r < reRegister; r++ {
				if err := server.RegisterRaw(method, handler); err != nil {
					t.Errorf("RegisterRaw(%s): %v", method, err)
					return
				}
			}
		}()
	}

	// Readers: hammer CallRaw across the whole method set until every writer is
	// done. This is the read side of the race (the Serve dispatch closure under
	// s.mu.RLock). A method not yet registered legitimately returns "unknown
	// method"; any successful response must be exactly correct.
	readerWG.Add(readers)
	for c := 0; c < readers; c++ {
		c := c
		go func() {
			defer readerWG.Done()
			<-start
			for i := c; ; i++ {
				select {
				case <-done:
					return
				default:
				}
				method := methodName(i % methods)
				resp, err := client.CallRaw(ctx, method, payload)
				if err != nil {
					if strings.Contains(err.Error(), "unknown method") {
						continue // writer has not reached this method yet
					}
					t.Errorf("CallRaw(%s): unexpected error: %v", method, err)
					return
				}
				if got := string(resp); got != want(method, payload) {
					t.Errorf("CallRaw(%s): got %q, want %q", method, got, want(method, payload))
					return
				}
				okCalls.Add(1)
			}
		}()
	}

	close(start)      // fire writers + readers simultaneously
	registrars.Wait() // every RegisterRaw returned -> happens-before the gate below
	close(done)       // stop the reader storm
	readerWG.Wait()   // nothing touches client/server past this point

	// Deterministic gate: with every registration now visible (WaitGroup
	// established the happens-before), each method must dispatch correctly on the
	// still-running server. This is the hard proof that dynamically registered
	// methods become callable and route to their own handler.
	for i := 0; i < methods; i++ {
		method := methodName(i)
		resp, err := client.CallRaw(ctx, method, payload)
		if err != nil {
			t.Fatalf("post-registration CallRaw(%s): %v", method, err)
		}
		if got := string(resp); got != want(method, payload) {
			t.Fatalf("post-registration CallRaw(%s): got %q, want %q", method, got, want(method, payload))
		}
	}

	t.Logf("dispatched %d calls concurrently with %d writers x %d re-registrations across %d readers",
		okCalls.Load(), methods, reRegister, readers)
}

// TestZAPServeCloseConcurrent guards the server lifecycle: Serve runs in one
// goroutine while Close is called from another. The ZAPServer is built once at
// construction (listenZAP), so the server field is never mutated after the
// struct is published and Serve/Close cannot race on it. Close must always stop
// the accept loop (Serve returns nil) and never leak a spinning goroutine — a
// Close that wins the startup race must still set closed. Run with -race.
func TestZAPServeCloseConcurrent(t *testing.T) {
	for i := 0; i < 100; i++ {
		server, err := Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("Listen: %v", err)
		}
		serveErr := make(chan error, 1)
		go func() { serveErr <- server.Serve(context.Background()) }()
		// Close races Serve's startup. The accept loop must always unwind.
		if err := server.Close(); err != nil {
			t.Errorf("iter %d: Close: %v", i, err)
		}
		if err := <-serveErr; err != nil {
			t.Errorf("iter %d: Serve: %v", i, err)
		}
	}
}
