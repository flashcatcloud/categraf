.SILENT:
.PHONY: build build-linux build-windows pack

APP:=categraf
ROOT:=$(shell pwd -P)
GIT_COMMIT:=$(shell git --work-tree ${ROOT}  rev-parse 'HEAD^{commit}')
_GIT_VERSION:=$(shell git --work-tree ${ROOT} describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null)
GIT_VERSION:=$(shell echo "${_GIT_VERSION}"| sed "s/-g\([0-9a-f]\{14\}\)$$/+\1/")
TAR_TAG:=$(shell echo ${GIT_VERSION}| awk -F"-" '{print $$1}')
BUILD_VERSION:='flashcat.cloud/categraf/config.Version=$(GIT_VERSION)'
LDFLAGS:="-w -s -X $(BUILD_VERSION)"

vendor:
	GOPROXY=https://goproxy.cn go mod vendor

build:
	echo "Building version $(GIT_VERSION)"
	go build -ldflags $(LDFLAGS) -o $(APP)

build-linux:
	echo "Building version $(GIT_VERSION) for linux"
	GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP)

build-windows:
	echo "Building version $(GIT_VERSION) for windows"
	GOOS=windows GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP).exe

build-mac:
	echo "Building version $(GIT_VERSION) for mac"
	GOOS=darwin GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP).mac

pack:build-linux build-windows
	rm -rf $(APP)-$(TAR_TAG).tar.gz
	rm -rf $(APP)-$(TAR_TAG).zip
	tar -zcvf $(APP)-$(TAR_TAG)-linux-amd64.tar.gz conf $(APP)
	zip -r $(APP)-$(TAR_TAG)-windows-amd64.zip conf $(APP).exe
