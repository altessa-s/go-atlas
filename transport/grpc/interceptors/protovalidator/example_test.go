// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// SimpleValidator demonstrates basic validation logic.
type SimpleValidator struct{}

func (v *SimpleValidator) Validate(_ context.Context, msg proto.Message) error {
	// Get the message reflection info
	msgReflect := msg.ProtoReflect()
	msgDesc := msgReflect.Descriptor()

	// Iterate through all fields and validate
	fields := msgDesc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Skip validation for fields that are not set
		if !msgReflect.Has(field) {
			// Check if field is required (in proto2)
			if field.Cardinality() == protoreflect.Required {
				return status.Errorf(codes.InvalidArgument,
					"required field %s is not set", field.Name())
			}
			continue
		}

		value := msgReflect.Get(field)

		// Validate string fields
		if field.Kind() == protoreflect.StringKind {
			str := value.String()

			// Example: check for empty strings in required fields
			if str == "" && field.Name() == "name" {
				return status.Errorf(codes.InvalidArgument,
					"field %s cannot be empty", field.Name())
			}

			// Example: validate email format
			if field.Name() == "email" && !isValidEmail(str) {
				return status.Errorf(codes.InvalidArgument,
					"field %s must be a valid email", field.Name())
			}
		}

		// Validate numeric fields
		if field.Kind() == protoreflect.Int32Kind || field.Kind() == protoreflect.Int64Kind {
			num := value.Int()

			// Example: check for positive values
			if field.Name() == "age" && num <= 0 {
				return status.Errorf(codes.InvalidArgument,
					"field %s must be positive", field.Name())
			}
		}
	}

	return nil
}

func isValidEmail(email string) bool {
	// Simple email validation for example
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

// ValidateFunc demonstrates functional validator.
func ValidateFunc(_ context.Context, msg proto.Message) error {
	// Example functional validator
	msgReflect := msg.ProtoReflect()
	msgName := string(msgReflect.Descriptor().FullName())

	// Custom validation logic based on message type
	switch msgName {
	case "example.CreateUserRequest":
		return validateCreateUserRequest(msgReflect)
	case "example.UpdateUserRequest":
		return validateUpdateUserRequest(msgReflect)
	default:
		// No validation for unknown types
		return nil
	}
}

func validateCreateUserRequest(msg protoreflect.Message) error {
	// Example validation for CreateUserRequest
	nameField := msg.Descriptor().Fields().ByName("name")
	if nameField != nil && msg.Has(nameField) {
		name := msg.Get(nameField).String()
		if len(name) < 2 {
			return status.Error(codes.InvalidArgument, "name must be at least 2 characters")
		}
	}
	return nil
}

func validateUpdateUserRequest(msg protoreflect.Message) error {
	// Example validation for UpdateUserRequest
	idField := msg.Descriptor().Fields().ByName("id")
	if idField != nil && msg.Has(idField) {
		id := msg.Get(idField).String()
		if id == "" {
			return status.Error(codes.InvalidArgument, "id is required for updates")
		}
	}
	return nil
}

func ExampleServerInterceptor() {
	// Create validator
	validator := &SimpleValidator{}

	// Create interceptor
	interceptor := protovalidator.ServerInterceptor(validator)

	// Create server with proto validation
	server := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
		grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
	)

	// Register your service
	// pb.RegisterUserServiceServer(server, &UserService{})
	_ = server

	fmt.Println("Server configured with proto validation")
	// Output: Server configured with proto validation
}

func ExampleServerInterceptor_withIgnoreMethods() {
	validator := &SimpleValidator{}

	// Create interceptor that ignores specific methods
	interceptor := protovalidator.ServerInterceptor(validator,
		protovalidator.WithIgnoreMethods(
			"/grpc.health.v1.Health/Check",
			"/grpc.health.v1.Health/Watch",
		),
	)

	server := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
	)

	_ = server
	fmt.Println("Server configured with ignored methods")
	// Output: Server configured with ignored methods
}

func ExampleServerInterceptor_functionalValidator() {
	// Use functional validator for simple cases
	interceptor := protovalidator.ServerInterceptor(
		protovalidator.ValidatorFunc(ValidateFunc),
	)

	server := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
	)

	_ = server
	fmt.Println("Server configured with functional validator")
	// Output: Server configured with functional validator
}

func ExampleServerInterceptor_customValidator() {
	// Custom validator with business logic
	businessValidator := protovalidator.ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		msgReflect := msg.ProtoReflect()
		msgName := string(msgReflect.Descriptor().FullName())

		// Apply different validation rules based on user context
		userRole, ok := ctx.Value("user_role").(string)
		if !ok {
			userRole = "guest"
		}

		// Stricter validation for non-admin users
		if userRole != "admin" && msgName == "example.CreateUserRequest" {
			// Additional validation for non-admin users
			emailField := msgReflect.Descriptor().Fields().ByName("email")
			if emailField != nil && msgReflect.Has(emailField) {
				email := msgReflect.Get(emailField).String()
				if !strings.HasSuffix(email, "@company.com") {
					return status.Error(codes.PermissionDenied,
						"non-admin users can only create accounts with company email")
				}
			}
		}

		return nil
	})

	interceptor := protovalidator.ServerInterceptor(businessValidator)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			// auth.ServerUnaryInterceptor(),          // Set user_role in context
			interceptor.ServerUnaryInterceptor(),
		),
	)

	_ = server
	fmt.Println("Server configured with role-based validation")
	// Output: Server configured with role-based validation
}

func ExampleServerInterceptor_withChain() {
	validator := &SimpleValidator{}

	// Create interceptor
	interceptor := protovalidator.ServerInterceptor(validator)

	// Example of using validator with other interceptors
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			// auth.ServerUnaryInterceptor(),        // Authenticate first
			interceptor.ServerUnaryInterceptor(), // Validate requests
			// logger.ServerUnaryInterceptor(),      // Log after validation
			// recovery.ServerUnaryInterceptor(),    // Recover from panics last
		),
	)

	_ = server
	fmt.Println("Server configured with interceptor chain")
	// Output: Server configured with interceptor chain
}

func ExampleServerInterceptor_realWorldUsage() {
	// Example showing how this would be used with real protobuf messages
	validator := protovalidator.ValidatorFunc(func(_ context.Context, msg proto.Message) error {
		// In real code, you would import your generated protobuf package
		// and perform validation based on the actual message types:

		// switch req := msg.(type) {
		// case *pb.CreateUserRequest:
		//     if req.Name == "" {
		//         return status.Error(codes.InvalidArgument, "name is required")
		//     }
		//     if !isValidEmail(req.Email) {
		//         return status.Error(codes.InvalidArgument, "invalid email format")
		//     }
		// case *pb.UpdateUserRequest:
		//     if req.Id == "" {
		//         return status.Error(codes.InvalidArgument, "id is required")
		//     }
		// }

		return nil
	})

	interceptor := protovalidator.ServerInterceptor(validator)

	server := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
	)

	_ = server
	fmt.Println("Example of real-world protobuf validation")
	// Output: Example of real-world protobuf validation
}
