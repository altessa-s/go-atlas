// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpc

import (
	"errors"
	"reflect"
	"slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrInvalidServiceRegistration indicates that a service descriptor could not
// be registered safely.
var ErrInvalidServiceRegistration = errors.New("invalid gRPC service registration")

type serviceRegistration struct {
	desc *grpc.ServiceDesc
	impl any
}

type serviceRegistrar struct {
	services []serviceRegistration

	// Reflection queries the live server for its service list after startup.
	reflection.ServiceInfoProvider
}

func (r *serviceRegistrar) RegisterService(desc *grpc.ServiceDesc, impl any) {
	if desc != nil {
		snapshot := *desc
		snapshot.Methods = slices.Clone(desc.Methods)
		snapshot.Streams = slices.Clone(desc.Streams)
		desc = &snapshot
	}
	r.services = append(r.services, serviceRegistration{desc: desc, impl: impl})
}

func (r *serviceRegistrar) validate() error {
	names := make(map[string]struct{}, len(r.services))
	for i, service := range r.services {
		if err := service.validate(names); err != nil {
			return coreerrs.Wrapf(err, "service registration %d", i+1)
		}
	}
	return nil
}

func (s serviceRegistration) validate(names map[string]struct{}) error {
	desc := s.desc
	if desc == nil {
		return coreerrs.Wrap(ErrInvalidServiceRegistration, "nil service descriptor")
	}
	if _, exists := names[desc.ServiceName]; exists {
		return coreerrs.Wrap(ErrInvalidServiceRegistration, "duplicate service name")
	}
	names[desc.ServiceName] = struct{}{}

	// grpc-go permits a nil implementation for handlers that dispatch without it.
	if desc.HandlerType != nil || s.impl != nil {
		handlerType := reflect.TypeOf(desc.HandlerType)
		if handlerType == nil || handlerType.Kind() != reflect.Pointer || handlerType.Elem().Kind() != reflect.Interface {
			return coreerrs.Wrap(ErrInvalidServiceRegistration, "HandlerType must be a pointer to an interface")
		}
		if implType := reflect.TypeOf(s.impl); implType != nil && !implType.Implements(handlerType.Elem()) {
			return coreerrs.Wrap(ErrInvalidServiceRegistration, "service implementation does not implement HandlerType")
		}
	}

	methodNames := make(map[string]struct{}, len(desc.Methods))
	for i, method := range desc.Methods {
		if method.Handler == nil {
			return coreerrs.Wrapf(ErrInvalidServiceRegistration, "method %d has a nil handler", i+1)
		}
		methodNames[method.MethodName] = struct{}{}
	}
	for i, stream := range desc.Streams {
		if stream.Handler == nil {
			return coreerrs.Wrapf(ErrInvalidServiceRegistration, "stream %d has a nil handler", i+1)
		}
		if _, exists := methodNames[stream.StreamName]; exists {
			return coreerrs.Wrapf(ErrInvalidServiceRegistration, "stream %d shares a name with a unary method", i+1)
		}
		if !stream.ClientStreams && !stream.ServerStreams {
			return coreerrs.Wrapf(ErrInvalidServiceRegistration, "stream %d has no streaming direction", i+1)
		}
	}
	return nil
}
