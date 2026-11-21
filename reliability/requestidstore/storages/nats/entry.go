// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var codec = serializer.NewJSON()

// Store token bytes explicitly: Go strings can contain non-UTF-8 data.
type record struct {
	requestid.Entry
	Token []byte
}

func encode(e requestid.Entry) ([]byte, error) {
	data, err := codec.Serialize(record{Entry: e, Token: []byte(e.Token)})
	return data, coreerrs.WrapOperation(err, "encode request ID entry")
}

func decode(data []byte) (requestid.Entry, error) {
	var r record
	err := codec.Deserialize(data, &r)
	r.Entry.Token = string(r.Token)
	return r.Entry, coreerrs.WrapOperation(err, "decode request ID entry")
}
