// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"encoding/json"
)

// JSONCodec is a JSON-based codec
type JSONCodec struct{}

func (JSONCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Decode(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// defaultCodec is used when no codec is specified
var defaultCodec Codec = JSONCodec{}

// BinaryCodec passes bytes through unchanged (for pre-encoded data)
type BinaryCodec struct{}

func (BinaryCodec) Encode(v interface{}) ([]byte, error) {
	if b, ok := v.([]byte); ok {
		return b, nil
	}
	if b, ok := v.(*[]byte); ok {
		return *b, nil
	}
	return json.Marshal(v)
}

func (BinaryCodec) Decode(data []byte, v interface{}) error {
	if b, ok := v.(*[]byte); ok {
		*b = data
		return nil
	}
	return json.Unmarshal(data, v)
}

// Binary is a codec that passes bytes through unchanged
var Binary Codec = BinaryCodec{}
