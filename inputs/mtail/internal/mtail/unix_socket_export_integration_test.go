// Copyright 2019 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package mtail_test

import (
	"log"
	"net"
	"path/filepath"
	"testing"

	"flashcat.cloud/categraf/inputs/mtail/internal/mtail"
	"flashcat.cloud/categraf/inputs/mtail/internal/testutil"
)

func TestBasicUNIXSockets(t *testing.T) {
	testutil.SkipIfShort(t)
	tmpDir := testutil.TestTempDir(t)
	sockListenAddr := filepath.Join(tmpDir, "mtail_test.sock")

	_, stopM := mtail.TestStartServer(t, 1, mtail.LogPathPatterns(tmpDir+"/*"), mtail.ProgramPath("../../examples/linecount.mtail"), mtail.BindUnixSocket(sockListenAddr))
	defer stopM()

	log.Println("check that server is listening")

	addr, err := net.ResolveUnixAddr("unix", sockListenAddr)
	testutil.FatalIfErr(t, err)
	_, err = net.DialUnix("unix", nil, addr)
	testutil.FatalIfErr(t, err)
}
