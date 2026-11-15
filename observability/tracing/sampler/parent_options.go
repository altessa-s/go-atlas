// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

//go:generate go tool optgen generate --type=parentBasedOptions --option-type=ParentBasedOption --output=parent_options_gen.go

// parentBasedOptions configures [NewParentBased]: the sampler consulted for
// each kind of parent. Root spans always use the root sampler passed to
// [NewParentBased].
type parentBasedOptions struct {
	// remoteParentSampled ([WithRemoteParentSampled]) decides spans whose
	// remote parent was sampled.
	remoteParentSampled Sampler `optgen:"default=AlwaysOn()"`
	// remoteParentNotSampled ([WithRemoteParentNotSampled]) decides spans
	// whose remote parent was not sampled.
	remoteParentNotSampled Sampler `optgen:"default=AlwaysOff()"`
	// localParentSampled ([WithLocalParentSampled]) decides spans whose local
	// parent was sampled.
	localParentSampled Sampler `optgen:"default=AlwaysOn()"`
	// localParentNotSampled ([WithLocalParentNotSampled]) decides spans whose
	// local parent was not sampled.
	localParentNotSampled Sampler `optgen:"default=AlwaysOff()"`
}
