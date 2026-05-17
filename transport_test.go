package rpc

import (
	"context"
	"strings"
	"testing"
)

func TestTransportRegistryGatesGRPC(t *testing.T) {
	// Default build must always have ZAP
	if !HasTransport(TransportZAP) {
		t.Fatal("ZAP transport must always be registered")
	}
	// gRPC is only present when built with -tags=grpc
	if HasTransport(TransportGRPC) {
		t.Log("grpc transport present (built with -tags=grpc)")
	} else {
		t.Log("grpc transport absent (default build)")
		// Dialing grpc on default build must return helpful error
		_, err := Dial(context.Background(), "localhost:1234", WithTransport(TransportGRPC))
		if err == nil {
			t.Fatal("expected error when dialing unregistered transport")
		}
		if !strings.Contains(err.Error(), "-tags=grpc") {
			t.Fatalf("error must hint at build tag, got: %v", err)
		}
	}
}
