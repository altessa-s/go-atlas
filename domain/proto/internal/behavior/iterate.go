// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import "google.golang.org/protobuf/reflect/protoreflect"

// ForEachMessageInList invokes fn for each message element of list. Iteration
// stops at the first non-nil error from fn; lists with no elements are a
// no-op.
//
// The helper centralizes the canonical "for i := range list.Len();
// list.Get(i).Message()" loop used by both the field-mask walker and the
// field-behavior stripper. Callers must ensure the list's element kind is
// MessageKind — list.Get(i).Message() on a scalar list returns an invalid
// message and fn would observe that.
func ForEachMessageInList(list protoreflect.List, fn func(i int, elem protoreflect.Message) error) error {
	for i := range list.Len() {
		if err := fn(i, list.Get(i).Message()); err != nil {
			return err
		}
	}
	return nil
}
