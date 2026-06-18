//go:build !no_logs && windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

type windowsFileID struct {
	volumeSerialNumber uint32
	fileIndexHigh      uint32
	fileIndexLow       uint32
}

// DidRotate returns true if the file has been log-rotated.
// When a log rotation occurs, the file can be either:
// - renamed and recreated
// - removed and recreated
// - truncated
func DidRotate(file *os.File, lastReadOffset int64) (bool, error) {
	if file == nil {
		return false, nil
	}

	currentFile, err := openFile(file.Name())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer currentFile.Close()

	currentInfo, err := currentFile.Stat()
	if err != nil {
		return false, err
	}

	oldInfo, err := file.Stat()
	if err != nil {
		return true, nil
	}

	truncated := currentInfo.Size() < lastReadOffset || oldInfo.Size() < lastReadOffset
	if truncated {
		return true, nil
	}

	oldID, err := getWindowsFileID(file)
	if err != nil {
		return false, err
	}
	currentID, err := getWindowsFileID(currentFile)
	if err != nil {
		return false, err
	}

	return oldID != currentID, nil
}

func getWindowsFileID(file *os.File) (windowsFileID, error) {
	var info windows.ByHandleFileInformation
	err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info)
	if err != nil {
		return windowsFileID{}, err
	}
	runtime.KeepAlive(file)

	return windowsFileID{
		volumeSerialNumber: info.VolumeSerialNumber,
		fileIndexHigh:      info.FileIndexHigh,
		fileIndexLow:       info.FileIndexLow,
	}, nil
}
