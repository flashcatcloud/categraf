//go:build !no_logs && windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

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

	log.Println("Opening ", t.fullpath)
	f, err := openFile(t.fullpath)
	if err != nil {
		return err
	}
	filePos, err := f.Seek(offset, whence)
	if err != nil {
		f.Close()
		return err
	}

	// Keep this handle open for rotation detection and to drain the old file
	// after it has been renamed. Normal reads open the path briefly so they
	// keep following the current file.
	t.osFile = f
	t.readOffset = filePos
	t.decodedOffset = filePos

	return nil
}

func (t *Tailer) readAvailable() (int, error) {
	if atomic.LoadInt32(&t.didFileRotate) != 0 && t.osFile != nil {
		return t.readFromFile(t.osFile, false)
	}

	f, err := openFile(t.fullpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return t.readFromFile(f, true)
}

func (t *Tailer) readFromFile(f *os.File, resetOffsetOnTruncate bool) (int, error) {
	st, err := f.Stat()
	if err != nil {
		log.Println("Error stat()ing file", err)
		return 0, err
	}

	sz := st.Size()
	offset := t.GetReadOffset()
	if resetOffsetOnTruncate && sz == 0 {
		log.Println("File size now zero, resetting offset")
		t.SetReadOffset(0)
		t.SetDecodedOffset(0)
	} else if resetOffsetOnTruncate && sz < offset {
		log.Println("Offset off end of file, resetting")
		t.SetReadOffset(0)
		t.SetDecodedOffset(0)
	}
	if _, err := f.Seek(t.GetReadOffset(), io.SeekStart); err != nil {
		log.Println("Error seeking file", err)
		return 0, err
	}
	bytes := 0

	for {
		inBuf := make([]byte, 4096)
		n, err := f.Read(inBuf)
		bytes += n
		if n == 0 || err != nil {
			return bytes, err
		}
		t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
	}
}

// read lets the tailer tail the content of a file until it is closed. The
// windows version open and close the file between each call to 'read'. This is
// needed in order not to block the file and prevent the user from renaming it.
func (t *Tailer) read() (int, error) {
	n, err := t.readAvailable()
	if err == io.EOF || os.IsNotExist(err) {
		return n, nil
	} else if err != nil {
		t.file.Source.Status.Error(err)
		return n, fmt.Errorf("Err: %s", err)
	}
	return n, nil
}
