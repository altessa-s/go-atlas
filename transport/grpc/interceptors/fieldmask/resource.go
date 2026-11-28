// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"strings"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	pbfieldmask "github.com/altessa-s/go-aip/fieldmask"
)

const (
	readMaskFieldName      = "read_mask"
	invalidReadMaskMessage = "Invalid read field mask"
	invalidReadMaskPath    = "invalid field mask path"
)

// readResource resolves, once per method, the resource a resource-relative
// read mask of method addresses: the element of the output's single repeated
// message field for a collection method, else the output itself. It is nil
// when the method is not in the global registry.
func (i *interceptor) readResource(method string) protoreflect.MessageDescriptor {
	if cached, ok := i.resources.Load(method); ok {
		md, _ := cached.(protoreflect.MessageDescriptor)
		return md
	}
	var resource protoreflect.MessageDescriptor
	if output := methodOutput(method); output != nil {
		resource = output
		if items := itemsField(output); items != nil && collectionMethod(method) {
			resource = items.Message()
		}
	}
	cached, _ := i.resources.LoadOrStore(method, resource)
	md, _ := cached.(protoreflect.MessageDescriptor)
	return md
}

// collectionMethod reports a List, Search or BatchGet method by its short name.
func collectionMethod(method string) bool {
	short := shortMethod(method)
	return strings.HasPrefix(short, "List") || strings.HasPrefix(short, "Search") || strings.HasPrefix(short, "BatchGet")
}

// methodOutput returns the output message of a "/package.Service/Method" name
// from the global registry, or nil when it is not registered.
func methodOutput(method string) protoreflect.MessageDescriptor {
	service, name, ok := strings.Cut(strings.TrimPrefix(method, "/"), "/")
	if !ok {
		return nil
	}
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil
	}
	md := sd.Methods().ByName(protoreflect.Name(name))
	if md == nil {
		return nil
	}
	return md.Output()
}

// itemsField returns the single repeated message field of a collection
// response, or nil when it has none or several.
func itemsField(md protoreflect.MessageDescriptor) protoreflect.FieldDescriptor {
	var items protoreflect.FieldDescriptor
	fields := md.Fields()
	for idx := range fields.Len() {
		f := fields.Get(idx)
		if !f.IsList() || f.Message() == nil {
			continue
		}
		if items != nil {
			return nil
		}
		items = f
	}
	return items
}

// resourcePathsValid reports whether the resource holds every path. "*"
// alone selects every field; an unregistered method is checked after the
// handler, by its response.
func (ri *requestInterceptor) resourcePathsValid(mask []string) bool {
	resource := ri.readResource(ri.meta.FullyMethodName)
	if resource == nil {
		return true
	}
	for _, p := range mask {
		if p != "*" && pbfieldmask.ResolvePath(resource, p) == "" {
			return false
		}
	}
	return true
}

// filterResources applies msk to every resource of a collection response and
// leaves its other fields untouched. Any other response is filtered whole.
func (ri *requestInterceptor) filterResources(resp proto.Message, msk pbfieldmask.FieldMask) error {
	m := resp.ProtoReflect()
	items := itemsField(m.Descriptor())
	if items == nil || !collectionMethod(ri.meta.FullyMethodName) {
		return msk.Filter(resp)
	}
	list := m.Get(items).List()
	for idx := range list.Len() {
		if err := msk.Filter(list.Get(idx).Message().Interface()); err != nil {
			return err
		}
	}
	return nil
}

// resourceMaskError is the refusal of an invalid resource-relative read
// mask: the default refusal with a violation on the read mask field.
func (ri *requestInterceptor) resourceMaskError() error {
	field := readMaskFieldName
	if ri.opts.maskFieldName != "" {
		field = ri.opts.maskFieldName
	}
	st, err := status.New(codes.InvalidArgument, invalidReadMaskMessage).WithDetails(
		&errdetails.ErrorInfo{Reason: invalidFieldMaskReason},
		&badrequestv1.BadRequest{FieldViolations: []*badrequestv1.FieldViolation{{FieldPath: new(field), Message: new(invalidReadMaskPath)}}},
	)
	if err != nil {
		return invalidReadMaskStatus.Err()
	}
	return st.Err()
}
