// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package updatevalidation

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const maxResourceDepth = 8

// ResourcePath returns the singular message fields through which request holds
// resource, identified by instance. An empty path means resource is request.
func ResourcePath(request, resource proto.Message) ([]protoreflect.FieldDescriptor, bool) {
	if request == resource {
		return nil, true
	}
	if request == nil || resource == nil {
		return nil, false
	}
	return resourcePath(request.ProtoReflect(), resource, maxResourceDepth)
}

func resourcePath(msg protoreflect.Message, resource proto.Message, depth int) ([]protoreflect.FieldDescriptor, bool) {
	if depth == 0 {
		return nil, false
	}
	var path []protoreflect.FieldDescriptor
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if fd.Message() == nil || fd.IsList() || fd.IsMap() {
			return true
		}
		child := value.Message()
		if child.Interface() == resource {
			path = []protoreflect.FieldDescriptor{fd}
			return false
		}
		if tail, ok := resourcePath(child, resource, depth-1); ok {
			path = append([]protoreflect.FieldDescriptor{fd}, tail...)
			return false
		}
		return true
	})
	return path, path != nil
}
