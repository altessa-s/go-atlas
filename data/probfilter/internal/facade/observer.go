// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade

import (
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

// ObserverSlot holds the [probfilter.Observer] of a filter facade and reports
// operations to it. It is safe for concurrent use; the zero value holds no
// observer, in which case every report costs a single atomic load.
type ObserverSlot struct {
	p atomic.Pointer[observerBox]
}

// observerBox lets an interface value live behind an atomic pointer.
type observerBox struct {
	o probfilter.Observer
}

// Set installs o; nil removes the current observer.
func (s *ObserverSlot) Set(o probfilter.Observer) {
	if o == nil {
		s.p.Store(nil)
		return
	}
	s.p.Store(&observerBox{o: o})
}

// Load returns the installed observer, or nil.
func (s *ObserverSlot) Load() probfilter.Observer {
	if b := s.p.Load(); b != nil {
		return b.o
	}
	return nil
}

// Lookup runs lookup and reports its outcome and duration.
func (s *ObserverSlot) Lookup(lookup func() (bool, error)) (bool, error) {
	o := s.Load()
	if o == nil {
		return lookup()
	}
	start := time.Now()
	found, err := lookup()
	o.ObserveLookup(found, err, time.Since(start))
	return found, err
}

// Added reports n successfully added values.
func (s *ObserverSlot) Added(n int) {
	if o := s.Load(); o != nil {
		o.ObserveAdd(n)
	}
}

// Rebuild runs rebuild and reports its duration and error.
func (s *ObserverSlot) Rebuild(rebuild func() error) error {
	o := s.Load()
	if o == nil {
		return rebuild()
	}
	start := time.Now()
	err := rebuild()
	o.ObserveRebuild(time.Since(start), err)
	return err
}
