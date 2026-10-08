// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projection_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/data/projection"
)

func Example() {
	parser, _ := projection.NewParser()
	spec, _ := parser.Parse(context.Background(), "name, createTime, address.city")

	policy, err := projection.NewTranslatorContext(
		projection.WithUntrustedInput(),
		projection.WithAllowedFields("name", "createTime", "address.*", "credentials.*"),
		projection.WithDeniedFields("credentials.password"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
		projection.WithRequiredFields("_id"),
		// The allow-list root "credentials" carries the denied password, so
		// the empty-request projection has to be listed explicitly.
		projection.WithDefaultFields("name", "createTime"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	sel, _ := policy.Resolve(spec)
	fmt.Println(sel.Include)

	all, _ := policy.Resolve(parser.MustParse(""))
	fmt.Println(all.Include)

	_, err = policy.Resolve(parser.MustParse("credentials"))
	fmt.Println(errors.Is(err, projection.ErrFieldNotAllowed))
	// Output:
	// [_id address.city created_at name]
	// [_id created_at name]
	// true
}
