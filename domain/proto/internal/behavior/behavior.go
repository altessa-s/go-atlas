// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"slices"
	"sync"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldCache memoizes resolved field_behavior annotations per field
// descriptor. Descriptors are process-lifetime singletons for compiled
// protos, so cardinality is bounded by the schema and extension decoding
// runs once per field instead of once per observation.
var fieldCache sync.Map // protoreflect.FieldDescriptor → []annotations.FieldBehavior

// Get returns every google.api.field_behavior value attached to fd. It returns
// nil when the annotation is absent. The slice is shared across callers and
// must not be modified.
func Get(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
	if v, ok := fieldCache.Load(fd); ok {
		return v.([]annotations.FieldBehavior) //nolint:errcheck // type guaranteed by Store
	}

	behaviors := resolve(fd)
	fieldCache.Store(fd, behaviors)

	return behaviors
}

// resolve decodes the field_behavior extension from the descriptor options.
func resolve(fd protoreflect.FieldDescriptor) []annotations.FieldBehavior {
	opts := fd.Options()
	if opts == nil {
		return nil
	}

	behaviors, ok := proto.GetExtension(opts, annotations.E_FieldBehavior).([]annotations.FieldBehavior)
	if !ok || len(behaviors) == 0 {
		return nil
	}

	return behaviors
}

// Has reports whether fd carries the given behavior.
func Has(fd protoreflect.FieldDescriptor, b annotations.FieldBehavior) bool {
	return slices.Contains(Get(fd), b)
}

// HasAny reports whether fd carries any of the given behaviors. With an empty
// set it returns false.
func HasAny(fd protoreflect.FieldDescriptor, set ...annotations.FieldBehavior) bool {
	if len(set) == 0 {
		return false
	}

	for _, got := range Get(fd) {
		if slices.Contains(set, got) {
			return true
		}
	}

	return false
}
