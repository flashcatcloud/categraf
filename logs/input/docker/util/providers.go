// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"log"
)

// ContainerImpl without implementation
// Implementations should call Register() in their init()
var containerImpl ContainerImplementation

// ContainerImpl returns the ContainerImplementation
func ContainerImpl() ContainerImplementation {
	if containerImpl == nil {
		panic("Trying to get nil ContainerInterface")
	}

	return containerImpl
}

// Register allows to set a ContainerImplementation
func Register(impl ContainerImplementation) {
	if containerImpl == nil {
		containerImpl = impl
	} else {
		log.Println("Trying to set multiple ContainerImplementation")
	}
}

// Deregister allows to unset a ContainerImplementation
// this should only be used in tests to clean the global state
func Deregister() {
	containerImpl = nil
}
