// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrZAPClosed      = errors.New("zap: connection closed")
	ErrZAPTimeout     = errors.New("zap: request timeout")
	ErrZAPInvalidResp = errors.New("zap: invalid response")
)

// MessageType identifies ZAP message types
type MessageType uint8

const (
	MsgRequest  MessageType = 0x01
	MsgResponse MessageType = 0x02
	MsgError    MessageType = 0x03
	MsgNotify   MessageType = 0x04
)

// ZAPConn represents a ZAP connection for RPC
type ZAPConn struct {
	conn     net.Conn
	writeMu  sync.Mutex
	pending  sync.Map // requestID -> chan *ZAPResponse
	nextID   atomic.Uint32
	closed   atomic.Bool
	readDone chan struct{}
}

// ZAPResponse holds a response from a ZAP call
type ZAPResponse struct {
	Data []byte
	Err  error
}

// ZAPDial connects to a ZAP server
func ZAPDial(ctx context.Context, addr string) (*ZAPConn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("zap dial: %w", err)
	}

	zc := &ZAPConn{
		conn:     conn,
		readDone: make(chan struct{}),
	}
	go zc.readLoop()
	return zc, nil
}

// Call makes a ZAP RPC call
func (z *ZAPConn) Call(ctx context.Context, method string, payload []byte) ([]byte, error) {
	if z.closed.Load() {
		return nil, ErrZAPClosed
	}

	requestID := z.nextID.Add(1)
	respCh := make(chan *ZAPResponse, 1)
	z.pending.Store(requestID, respCh)
	defer z.pending.Delete(requestID)

	// Encode: [4 len][1 type][4 reqID][2 methodLen][method][payload]
	methodBytes := []byte(method)
	msgLen := 1 + 4 + 2 + len(methodBytes) + len(payload)

	buf := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(msgLen))
	buf[4] = byte(MsgRequest)
	binary.BigEndian.PutUint32(buf[5:9], requestID)
	binary.BigEndian.PutUint16(buf[9:11], uint16(len(methodBytes)))
	copy(buf[11:], methodBytes)
	copy(buf[11+len(methodBytes):], payload)

	z.writeMu.Lock()
	_, err := z.conn.Write(buf)
	z.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("zap write: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Err != nil {
			return nil, resp.Err
		}
		return resp.Data, nil
	case <-z.readDone:
		return nil, ErrZAPClosed
	}
}

// Notify sends a one-way notification (no response expected)
func (z *ZAPConn) Notify(ctx context.Context, method string, payload []byte) error {
	if z.closed.Load() {
		return ErrZAPClosed
	}

	methodBytes := []byte(method)
	msgLen := 1 + 2 + len(methodBytes) + len(payload)

	buf := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(msgLen))
	buf[4] = byte(MsgNotify)
	binary.BigEndian.PutUint16(buf[5:7], uint16(len(methodBytes)))
	copy(buf[7:], methodBytes)
	copy(buf[7+len(methodBytes):], payload)

	z.writeMu.Lock()
	_, err := z.conn.Write(buf)
	z.writeMu.Unlock()
	return err
}

func (z *ZAPConn) readLoop() {
	defer close(z.readDone)

	header := make([]byte, 4)
	for {
		if _, err := io.ReadFull(z.conn, header); err != nil {
			return
		}

		msgLen := binary.BigEndian.Uint32(header)
		if msgLen == 0 || msgLen > 64*1024*1024 { // 64MB max
			return
		}

		msg := make([]byte, msgLen)
		if _, err := io.ReadFull(z.conn, msg); err != nil {
			return
		}

		if len(msg) < 5 {
			continue
		}

		msgType := MessageType(msg[0])
		requestID := binary.BigEndian.Uint32(msg[1:5])
		payload := msg[5:]

		if ch, ok := z.pending.Load(requestID); ok {
			respCh := ch.(chan *ZAPResponse)
			switch msgType {
			case MsgResponse:
				respCh <- &ZAPResponse{Data: payload}
			case MsgError:
				respCh <- &ZAPResponse{Err: errors.New(string(payload))}
			}
		}
	}
}

// Close closes the connection
func (z *ZAPConn) Close() error {
	if z.closed.Swap(true) {
		return nil
	}
	return z.conn.Close()
}

// ZAPServer handles incoming ZAP RPC requests
type ZAPServer struct {
	listener net.Listener
	handler  ZAPHandler
	conns    sync.Map
	closed   atomic.Bool
}

// ZAPHandler handles ZAP requests
type ZAPHandler interface {
	HandleZAP(ctx context.Context, method string, payload []byte) ([]byte, error)
}

// ZAPHandlerFunc is a function adapter for ZAPHandler
type ZAPHandlerFunc func(ctx context.Context, method string, payload []byte) ([]byte, error)

func (f ZAPHandlerFunc) HandleZAP(ctx context.Context, method string, payload []byte) ([]byte, error) {
	return f(ctx, method, payload)
}

// NewZAPServer creates a new ZAP server
func NewZAPServer(listener net.Listener, handler ZAPHandler) *ZAPServer {
	return &ZAPServer{
		listener: listener,
		handler:  handler,
	}
}

// Serve starts serving requests
func (s *ZAPServer) Serve(ctx context.Context) error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			continue
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *ZAPServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	s.conns.Store(conn, struct{}{})
	defer s.conns.Delete(conn)

	header := make([]byte, 4)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}

		msgLen := binary.BigEndian.Uint32(header)
		if msgLen == 0 || msgLen > 64*1024*1024 {
			return
		}

		msg := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, msg); err != nil {
			return
		}

		if len(msg) < 1 {
			continue
		}

		msgType := MessageType(msg[0])

		switch msgType {
		case MsgRequest:
			if len(msg) < 7 {
				continue
			}
			requestID := binary.BigEndian.Uint32(msg[1:5])
			methodLen := binary.BigEndian.Uint16(msg[5:7])
			if len(msg) < 7+int(methodLen) {
				continue
			}
			method := string(msg[7 : 7+methodLen])
			payload := msg[7+methodLen:]

			go func() {
				respData, err := s.handler.HandleZAP(ctx, method, payload)
				s.sendResponse(conn, requestID, respData, err)
			}()

		case MsgNotify:
			if len(msg) < 3 {
				continue
			}
			methodLen := binary.BigEndian.Uint16(msg[1:3])
			if len(msg) < 3+int(methodLen) {
				continue
			}
			method := string(msg[3 : 3+methodLen])
			payload := msg[3+methodLen:]
			go s.handler.HandleZAP(ctx, method, payload)
		}
	}
}

func (s *ZAPServer) sendResponse(conn net.Conn, requestID uint32, data []byte, err error) {
	var msgType MessageType
	var payload []byte
	if err != nil {
		msgType = MsgError
		payload = []byte(err.Error())
	} else {
		msgType = MsgResponse
		payload = data
	}

	msgLen := 1 + 4 + len(payload)
	buf := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(msgLen))
	buf[4] = byte(msgType)
	binary.BigEndian.PutUint32(buf[5:9], requestID)
	copy(buf[9:], payload)

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	conn.Write(buf)
}

// Close closes the server
func (s *ZAPServer) Close() error {
	s.closed.Store(true)
	s.conns.Range(func(key, _ interface{}) bool {
		key.(net.Conn).Close()
		return true
	})
	return s.listener.Close()
}

// Addr returns the listener address
func (s *ZAPServer) Addr() net.Addr {
	return s.listener.Addr()
}
