.PHONY: start build

APP = categraf
VER = 0.1.1

all: build

build:
	go build -ldflags "-w -s -X flashcat.cloud/categraf/config.Version=$(VER)"

pack:
	env GOOS=linux GOARCH=amd64 go build -ldflags "-w -s -X flashcat.cloud/categraf/config.VERSION=$(VER)"
	env GOOS=windows GOARCH=amd64 go build -ldflags "-w -s -X flashcat.cloud/categraf/config.VERSION=$(VER)"
	rm -rf $(APP)-$(VER).tar.gz
	rm -rf $(APP)-$(VER).zip
	tar -zcvf $(APP)-$(VER)-linux-amd64.tar.gz conf $(APP)
	zip -r $(APP)-$(VER)-windows-amd64.zip conf $(APP).exe
