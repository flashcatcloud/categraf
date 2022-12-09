//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sync"
	"time"

	logsconfig "flashcat.cloud/categraf/config/logs"
)

// NOTE: to avoid races, do not modify the contents of the `expectedTags`
// slice, as those contents are referenced without holding the lock.

type localProvider struct {
	tags                 []string
	expectedTags         []string
	expectedTagsDeadline time.Time
	sync.RWMutex
}

// NewLocalProvider returns a new local Provider.
func NewLocalProvider(t []string) Provider {
	p := &localProvider{
		tags:         t,
		expectedTags: t,
	}

	if logsconfig.IsExpectedTagsSet() {
		// TODO
		// p.expectedTags = append(p.tags, "hosttags:testing")
		p.expectedTagsDeadline = time.Now().Add(time.Duration(1) * time.Second)

		// reset submitExpectedTags after deadline elapsed
		go func() {
			<-time.After(time.Until(p.expectedTagsDeadline))

			p.Lock()
			defer p.Unlock()
			p.expectedTags = nil
		}()
	}

	return p
}

// GetTags returns the list of locally-configured tags.  This will include the
// expected tags until the expected-tags deadline, if those are configured.  The
// returned slice is shared and must not be mutated.
func (p *localProvider) GetTags() []string {
	p.RLock()
	defer p.RUnlock()

	if p.expectedTags != nil {
		return p.expectedTags
	}
	return p.tags
}
