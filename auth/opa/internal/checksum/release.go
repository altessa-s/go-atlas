// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package checksum

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidPath reports whether name is a portable slash-separated release path.
func ValidPath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, "\\:\x00") && strings.TrimSpace(name) == name
}

// ValidDigest reports whether digest is a lowercase SHA-256 hex digest.
func ValidDigest(digest string) bool {
	return digestPattern.MatchString(digest)
}

// DecodeRelease decodes a flat file map or a release pointer with a root and file map.
func DecodeRelease[T any](content []byte) (string, map[string]T, error) {
	fields, err := decodeReleaseObject[json.RawMessage](content)
	if err != nil {
		return "", nil, err
	}
	var root string
	rootJSON, filesJSON := fields["root"], fields["files"]
	// In the flat formats, values are all strings or all objects. The pointer
	// has both a string "root" and an object "files".
	if bytes.HasPrefix(rootJSON, []byte(`"`)) && bytes.HasPrefix(filesJSON, []byte("{")) {
		const pointerFieldCount = 2
		if len(fields) != pointerFieldCount {
			return "", nil, errors.New("unexpected release manifest fields")
		}
		if err = json.Unmarshal(rootJSON, &root); err != nil {
			return "", nil, err
		}
		if !ValidPath(root) {
			return "", nil, coreerrs.WrapOperation(coreerrs.ErrInvalidArgument, "invalid release root")
		}
		content = filesJSON
	}
	entries, err := decodeReleaseObject[T](content)
	return root, entries, err
}

func decodeReleaseObject[T any](content []byte) (map[string]T, error) {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if token, err := dec.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("release manifest must be a JSON object")
	}

	entries := make(map[string]T)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok || !ValidPath(name) {
			return nil, errors.New("invalid release manifest file name")
		}
		if _, exists := entries[name]; exists {
			return nil, fmt.Errorf("duplicate release manifest entry %q", name)
		}
		var entry T
		if err := dec.Decode(&entry); err != nil {
			return nil, err
		}
		entries[name] = entry
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected content after release manifest")
	}
	return entries, nil
}
