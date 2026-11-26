// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"errors"
	"slices"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func validateUpdate(validator protovalidate.Validator, msg proto.Message, filter protovalidate.Filter, sel *updatevalidation.Selection) error {
	required := requiredViolations{
		blocked:  make(map[requiredField]bool),
		seenPath: make(map[string]bool),
	}
	body := msg
	resource := msg.ProtoReflect()
	for _, fd := range sel.Route {
		resource = resource.Get(fd).Message()
	}
	if resource.Interface() != sel.Resource {
		return interceptors.NewError(status.New(codes.Internal, "Internal Error"), errors.New("updatemask: the update resource is not held by the request"))
	}
	prefix := make([]*validate.FieldPathElement, 0, len(sel.Route))
	for _, fd := range sel.Route {
		prefix = append(prefix, pathElement(fd))
	}

	var scope *validationScope
	if sel.Err == nil {
		violations, err := sel.Spec.ValidateResolved(sel.Resource, sel.Mask)
		if err != nil {
			return internalValidationError(err)
		}
		tree := sel.Tree()
		if updatevalidation.HasPartialMap(resource, tree) {
			body = proto.Clone(msg)
			resource = body.ProtoReflect()
			for _, fd := range sel.Route {
				resource = resource.Get(fd).Message()
			}
			updatevalidation.PruneMaps(resource, tree)
		}
		built, err := buildSelectedScope(resource, sel.Mask.Paths(), prefix, sel)
		if err != nil {
			return internalValidationError(err)
		}
		scope = &built
		if err := required.addResolvedViolations(resource, prefix, violations, sel); err != nil {
			return internalValidationError(err)
		}
	}
	if err := required.walk(resource, prefix, scope, sel, nil); err != nil {
		return internalValidationError(err)
	}

	var outside map[proto.Message]bool
	if scope != nil && len(sel.Route) != 0 {
		outside = make(map[proto.Message]bool)
		markOutside(body.ProtoReflect(), resource.Interface(), outside)
	}
	scoped := protovalidate.FilterFunc(func(owner protoreflect.Message, desc protoreflect.Descriptor) bool {
		inScope := scope == nil || outside[owner.Interface()] || scope.allows(owner, desc)
		return inScope && required.allows(owner, desc) && (filter == nil || filter.ShouldValidate(owner, desc))
	})
	err := validator.Validate(body, protovalidate.WithFilter(scoped))
	violations, err := appendValidationErrors(required.errors, err, nil, scope)
	if err != nil {
		return err
	}
	if scope != nil {
		for _, target := range scope.targets {
			err = validator.Validate(target.message.Interface(), protovalidate.WithFilter(scoped))
			violations, err = appendValidationErrors(violations, err, target.prefix, scope)
			if err != nil {
				return err
			}
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return &protovalidate.ValidationError{Violations: violations}
}

func internalValidationError(err error) error {
	return interceptors.NewError(status.New(codes.Internal, "Internal Error"), err)
}

func markOutside(msg protoreflect.Message, resource proto.Message, outside map[proto.Message]bool) {
	id := msg.Interface()
	if id == resource || outside[id] {
		return
	}
	outside[id] = true
	msg.Range(func(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		updatevalidation.RangeMessages(fd, value, func(child protoreflect.Message) {
			markOutside(child, resource, outside)
		})
		return true
	})
}

func appendValidationErrors(dst []*protovalidate.Violation, err error,
	prefix []*validate.FieldPathElement, scope *validationScope,
) ([]*protovalidate.Violation, error) {
	if err == nil {
		return dst, nil
	}
	ve, ok := errors.AsType[*protovalidate.ValidationError](err)
	if !ok {
		return nil, err
	}
	for _, violation := range ve.Violations {
		if len(prefix) > 0 {
			violation.Proto.SetField(validate.FieldPath_builder{
				Elements: slices.Concat(prefix, violation.Proto.GetField().GetElements()),
			}.Build())
		}
		if scope == nil || scope.keepsViolation(violation) {
			dst = append(dst, violation)
		}
	}
	return dst, nil
}
