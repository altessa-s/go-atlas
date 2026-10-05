// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

// Envelope validation errors; a command failing either is terminated.
var (
	errEmptyCommandID     = errors.New("saga engine: command has no saga id")
	errMissingCommandData = errors.New("saga engine: command has no data field")
)

// command is a decoded start command. Data holds the saga payload encoded with
// the engine's serializer.
type command struct {
	ID   string
	Data []byte
}

// wireCommand is the JSON envelope: {"id": "<saga id>", "data": "<base64>"}.
//
// Data is always present on the wire, even when the serializer produced zero
// bytes, so a missing field marks a malformed command rather than a zero T.
type wireCommand struct {
	ID   string  `json:"id"`
	Data *[]byte `json:"data"`
}

func encodeCommand(id string, data []byte) ([]byte, error) {
	if data == nil {
		data = []byte{} // A nil slice would encode as null, read back as missing.
	}
	return json.Marshal(wireCommand{ID: id, Data: &data})
}

func decodeCommand(b []byte) (command, error) {
	// encoding/json would silently replace invalid UTF-8 inside the ID with
	// U+FFFD, turning a malformed command into a different saga ID.
	if !utf8.Valid(b) {
		return command{}, ErrInvalidID
	}
	var w wireCommand
	if err := json.Unmarshal(b, &w); err != nil {
		return command{}, err
	}
	if w.ID == "" {
		return command{}, errEmptyCommandID
	}
	if w.Data == nil {
		return command{}, errMissingCommandData
	}
	return command{ID: w.ID, Data: *w.Data}, nil
}

// msgID is the JetStream de-duplication key. De-duplication is stream-wide, not
// per subject, so the key is namespaced by definition; the length prefix keeps
// the tuple unambiguous. It is base64url-encoded because header values are
// normalized on the wire (trailing whitespace trimmed, CR/LF replaced), which
// would otherwise map distinct saga IDs to one key.
func msgID(definition, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(len(definition)) + ":" + definition + ":" + id))
}
