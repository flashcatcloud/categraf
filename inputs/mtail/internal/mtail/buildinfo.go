// Copyright 2020 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package mtail

import (
	"fmt"
	"runtime"
)

// BuildInfo records the compile-time information for use when reporting the mtail version.
type BuildInfo struct {
	Version string
}

func (b BuildInfo) String() string {
	return fmt.Sprintf(
		"mtail version %s go version %s go arch %s go os %s",
		b.Version,
		runtime.Version(),
		runtime.GOARCH,
		runtime.GOOS,
	)
}
