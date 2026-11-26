// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"strings"
	"testing"
)

func TestOutboxFactoryConfigDependency(t *testing.T) {
	t.Parallel()
	walkProductionImports(t, func(file, pkg, dependency string) {
		if pkg != "factory/reliability/outbox" && !strings.HasPrefix(pkg, "factory/reliability/outbox/") {
			return
		}
		isConfig := dependency == module+"config" || strings.HasPrefix(dependency, module+"config/")
		if isConfig && dependency != module+"config/outbox" {
			t.Errorf("%s: outbox factories must use config/outbox, found %s", file, dependency)
		}
	})
}
