//go:build !no_logs && !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"io"
	"log"
	"path/filepath"

	coreconfig "flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.file.Path)
	if err != nil {
		return err
	}
	t.fullpath = fullpath

	// adds metadata to enable users to filter logs by filename
	t.tags = t.buildTailerTags()

	if coreconfig.Config.DebugMode {
		log.Println("I! Opening", t.file.Path, "for tailer key", t.file.GetScanKey())
	}
	f, err := openFile(fullpath)
	if err != nil {
		return err
	}

	t.osFile = f
	ret, _ := f.Seek(offset, whence)
	t.readOffset = ret
	t.decodedOffset = ret

	return nil
}

// read lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) read() (int, error) {
	// keep reading data from file
	inBuf := make([]byte, 4096)
	n, err := t.osFile.Read(inBuf)
	if err != nil && err != io.EOF {
		// an unexpected error occurred, stop the tailor
		t.file.Source.Status.Error(err)
		return 0, fmt.Errorf("E! Unexpected error occurred while reading file: ", err)
	}
	if n == 0 {
		return 0, nil
	}
	t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
	t.incrementReadOffset(n)
	return n, nil
}
