# Hanzo Rpc

## Overview
Go module: github.com/luxfi/rpc

## Tech Stack
- **Language**: Go

## Build & Run
```bash
go build ./...           # default: ZAP transport only, no gRPC
go test  ./...
go build -tags=grpc ./...  # opt in to gRPC transport
go test  -tags=grpc ./...
```

## Transports
ZAP is the default and only always-on transport. Alternative transports
are gated behind Go build tags so the default footprint stays minimal:

| Transport | Tag      | File              | Notes                          |
|-----------|----------|-------------------|--------------------------------|
| ZAP       | (always) | `zap.go`          | Zero-copy, default for HFT/VM. |
| gRPC      | `grpc`   | `dial_grpc.go`    | Pulls in `google.golang.org/grpc`. |

Dial/Listen consult an in-process transport registry; tagged files
register themselves via `init()`. Requesting a transport that was not
compiled in returns an explicit error pointing at the missing tag.

## Structure
```
rpc/
  LICENSE
  client.go
  codec.go
  dial.go
  dial_grpc.go
  doc.go
  go.mod
  go.sum
  json.go
  json2/
  json_test.go
  options.go
  requester.go
  transport.go
  zap.go
```

## Key Files
- `go.mod` -- Go module definition
