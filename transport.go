// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"sync"
)

// Transport types
const (
	TransportZAP  = "zap"  // Zero-copy, default
	TransportGRPC = "grpc" // Google RPC, requires build tag
	TransportJSON = "json" // JSON-RPC over HTTP
)

// DefaultTransport is the default transport type (ZAP)
const DefaultTransport = TransportZAP

type dialFunc func(ctx context.Context, addr string, o *dialOptions) (Client, error)
type listenFunc func(addr string, o *serverOptions) (Server, error)

var (
	transportsMu sync.RWMutex
	transports   = map[string]struct {
		dial   dialFunc
		listen listenFunc
	}{
		TransportZAP: {dialZAP, listenZAP},
	}
)

// registerTransport registers a new transport (used by build tags)
func registerTransport(name string, dial dialFunc, listen listenFunc) {
	transportsMu.Lock()
	defer transportsMu.Unlock()
	transports[name] = struct {
		dial   dialFunc
		listen listenFunc
	}{dial, listen}
}

// AvailableTransports returns list of available transport types
func AvailableTransports() []string {
	transportsMu.RLock()
	defer transportsMu.RUnlock()
	result := make([]string, 0, len(transports))
	for name := range transports {
		result = append(result, name)
	}
	return result
}

// HasTransport checks if a transport is available
func HasTransport(name string) bool {
	transportsMu.RLock()
	defer transportsMu.RUnlock()
	_, ok := transports[name]
	return ok
}
