// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

const (
	maxMetadataKeys      = 100
	maxMetadataKeySize   = 256
	maxMetadataValueSize = 1024
	maxMetadataValues    = 20
	storageKeyBytes      = 16
)

// ErrMetadataTooLarge means cache partition metadata exceeds the key-generation
// bounds. The interceptor skips caching when it cannot include all discriminators.
var ErrMetadataTooLarge = errors.New("cache: metadata exceeds key generation limits")

var errIdentityUnknown = errors.New("cache: verified identity has no cache namespace")

// MetadataProcessor extracts additional response discriminators from metadata
// or context values. Use [IdentityFunc] for verified caller identity.
type MetadataProcessor func(ctx context.Context, md metadata.MD) map[string]string

// DefaultMetadataKeys is the standard set of response discriminators. The
// identity headers are retained for services without authentication.
var DefaultMetadataKeys = []string{
	"organization-id",
	"org-id",
	"user-id",
	"tenant-id",
	"x-user-id",
	"x-org-id",
	"x-organization-id",
	"x-tenant-id",
	"x-client-id",
	"x-api-version",
	"x-feature-flags",
	fieldmask.DefaultMetadataReadMaskHeader,
}

// KeyGenerator generates a storage key from the context, method and request.
// The interceptor independently checks the full request tuple before serving
// an entry from that key.
type KeyGenerator func(ctx context.Context, method string, req any) (string, error)

// DefaultKeyGenerator generates a key over the method, verified identity,
// request bytes, and default metadata discriminators.
func DefaultKeyGenerator(ctx context.Context, method string, req any) (string, error) {
	return newDefaultKeyGenerator()(ctx, method, req)
}

func newDefaultKeyGenerator() KeyGenerator {
	return NewKeyGenerator(nil, nil)
}

// NewKeyGenerator creates a key generator with the given metadata keys and
// optional processor. An empty list uses [DefaultMetadataKeys]. The default
// read-mask header is always included.
func NewKeyGenerator(metadataKeys []string, processor MetadataProcessor) KeyGenerator {
	fields := newDiscriminators(metadataKeys, processor, "")
	return func(ctx context.Context, method string, req any) (string, error) {
		namespace, ok := cacheNamespace(ctx, nil, false)
		if !ok {
			return "", errIdentityUnknown
		}
		input, err := fields.keyInput(ctx, method, namespace, req)
		if err != nil {
			return "", err
		}
		return input.storageKey(), nil
	}
}

type keyInput struct {
	method, namespace string
	request, fields   []byte
}

func (in keyInput) storageKey() string {
	digest := in.digest("grpc-cache/key")
	return hex.EncodeToString(digest[:storageKeyBytes])
}

func (in keyInput) verifier() []byte {
	return in.digest("grpc-cache/verifier")
}

func (in keyInput) digest(domain string) []byte {
	return keys.Digest([]byte(domain), []byte(in.method), []byte(in.namespace), in.request, in.fields)
}

type discriminators struct {
	keys      []string
	processor MetadataProcessor
}

func newDiscriminators(metadataKeys []string, processor MetadataProcessor, readMaskHeader string) discriminators {
	if len(metadataKeys) == 0 {
		metadataKeys = DefaultMetadataKeys
	}

	relevant := make([]string, 0, len(metadataKeys)+2)
	for _, key := range metadataKeys {
		relevant = append(relevant, strings.ToLower(key))
	}
	relevant = append(relevant, fieldmask.DefaultMetadataReadMaskHeader)
	if readMaskHeader != "" {
		relevant = append(relevant, strings.ToLower(readMaskHeader))
	}
	slices.Sort(relevant)
	return discriminators{keys: slices.Compact(relevant), processor: processor}
}

func (d discriminators) keyInput(ctx context.Context, method, namespace string, req any) (keyInput, error) {
	data, err := marshalRequest(req)
	if err != nil {
		return keyInput{}, err
	}
	md, _ := metadata.FromIncomingContext(ctx)
	fields, err := appendMetadataFields(md, d.keys)
	if err != nil {
		return keyInput{}, err
	}
	if d.processor != nil {
		if md == nil {
			md = make(metadata.MD)
		}
		fields, err = appendProcessedFields(fields, d.processor(ctx, md))
		if err != nil {
			return keyInput{}, err
		}
	}
	return keyInput{method: method, namespace: namespace, request: data, fields: fields}, nil
}

func marshalRequest(req any) ([]byte, error) {
	if msg, ok := req.(proto.Message); ok {
		data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(msg)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal proto message")
		}
		return data, nil
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "marshal JSON request")
	}
	return data, nil
}

func appendMetadataFields(md metadata.MD, relevantKeys []string) ([]byte, error) {
	var fields []byte
	processedKeys := 0
	for _, key := range relevantKeys {
		values := md[key]
		if len(values) == 0 {
			continue
		}
		processedKeys++
		if processedKeys > maxMetadataKeys || len(key) > maxMetadataKeySize || len(values) > maxMetadataValues {
			return nil, ErrMetadataTooLarge
		}
		fields = appendField(fields, tagMetadataKey, key)
		for _, v := range values {
			if len(v) > maxMetadataValueSize {
				return nil, ErrMetadataTooLarge
			}
			fields = appendField(fields, tagMetadataValue, v)
		}
	}
	return fields, nil
}

func appendProcessedFields(fields []byte, extra map[string]string) ([]byte, error) {
	if len(extra) > maxMetadataKeys {
		return nil, ErrMetadataTooLarge
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		value := extra[name]
		if len(name) > maxMetadataKeySize || len(value) > maxMetadataValueSize {
			return nil, ErrMetadataTooLarge
		}
		fields = appendField(fields, tagProcessedKey, name)
		fields = appendField(fields, tagProcessedValue, value)
	}
	return fields, nil
}

const (
	tagMetadataKey byte = iota + 1
	tagMetadataValue
	tagProcessedKey
	tagProcessedValue
)

// Each field is framed by its kind and length so that a value containing a
// key or a separator cannot reproduce another tuple's encoding.
func appendField(fields []byte, tag byte, value string) []byte {
	fields = append(fields, tag)
	fields = binary.AppendUvarint(fields, uint64(len(value)))
	return append(fields, value...)
}
