.SILENT:
.PHONY: build build-linux build-windows pack

APP:=categraf
ROOT:=$(shell pwd -P)
GIT_COMMIT:=$(shell git --work-tree ${ROOT}  rev-parse 'HEAD^{commit}')
_GIT_VERSION:=$(shell git --work-tree ${ROOT} describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null)
TAG=$(shell echo "${_GIT_VERSION}" |  awk -F"-" '{print $$1}')
GIT_VERSION:="$(TAG)-$(GIT_COMMIT)"
BUILD_VERSION:='flashcat.cloud/categraf/config.Version=$(GIT_VERSION)'
LDFLAGS:="-w -s -X $(BUILD_VERSION)"
STYLE_CHECK_GOFILE  := $$(find . -name '*.go')
GO          := GO111MODULE=on go

all: build

vendor:
	GOPROXY=https://goproxy.cn go mod vendor

tidy-mod: 
	go mod tidy
vendor-ci: tidy-mod
	go mod vendor

build:
	echo "Building version $(GIT_VERSION)"
	go build -ldflags $(LDFLAGS) -o $(APP)

build-test:
	echo "Building version $(GIT_VERSION)"
	go build -ldflags $(LDFLAGS) -o $(APP)
	echo "Linux amd64 building version $(GIT_VERSION)"
	GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP)
	echo "Linux arm64 building version $(GIT_VERSION)"
	GOOS=linux GOARCH=arm64 go build -ldflags $(LDFLAGS) -o $(APP)
	echo "Windows amd64 building version $(GIT_VERSION)"
	GOOS=windows GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP).exe
	echo "Windows arm64 building version $(GIT_VERSION)"
	GOOS=windows GOARCH=arm64 go build -ldflags $(LDFLAGS) -o $(APP).exe
	echo "Linux amd64 slim building version $(GIT_VERSION)"
	GOOS=linux GOARCH=amd64 go build --tags "no_logs no_prometheus no_traces" -ldflags $(LDFLAGS) -o $(APP)
	echo "Linux arm64 slim building version $(GIT_VERSION)"
	GOOS=linux GOARCH=arm64 go build --tags "no_logs no_prometheus no_traces" -ldflags $(LDFLAGS) -o $(APP)

build-pure:
	echo "Building version $(GIT_VERSION)"
	go build --tags "no_prometheus no_traces" -ldflags $(LDFLAGS) -o $(APP)

build-linux:
	echo "Building version $(GIT_VERSION) for linux"
	GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP)

build-linux-arm:
	echo "Building version $(GIT_VERSION) for linux"
	GOOS=linux GOARCH=arm64 go build -ldflags $(LDFLAGS) -o $(APP)

build-windows:
	echo "Building version $(GIT_VERSION) for windows"
	GOOS=windows GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP).exe

build-mac:
	echo "Building version $(GIT_VERSION) for mac"
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags $(LDFLAGS) -o $(APP).mac

build-mac-arm:
	echo "Building version $(GIT_VERSION) for mac"
	GOOS=darwin GOARCH=arm64 go build -ldflags $(LDFLAGS) -o $(APP).mac

build-image: build-linux
	echo "Building image flashcatcloud/categraf:$(TAG)"
	cp -rf conf/config.toml conf/logs.toml conf/input.cpu conf/input.disk conf/input.diskio conf/input.docker conf/input.kernel conf/input.kernel_vmstat conf/input.linux_sysctl_fs conf/input.mem conf/input.net conf/input.netstat conf/input.processes conf/input.system docker/conf/ && cp -f categraf docker/ && cd docker && docker build -t flashcatcloud/categraf:$(TAG) .

pack:build-linux build-windows
	rm -rf $(APP)-$(TAG).tar.gz
	rm -rf $(APP)-$(TAG).zip
	tar -zcvf $(APP)-$(TAG)-linux-amd64.tar.gz conf $(APP)
	zip -r $(APP)-$(TAG)-windows-amd64.zip conf $(APP).exe

go-version-check:
	bash ./scripts/ci/go_version_check.sh

go-vet-check: tidy-mod
	bash ./scripts/ci/go_vet.sh
