.PHONY: start build

APP = categraf
VER = 0.1.0

all: build

build:
	go build -ldflags "-w -s -X flashcat.cloud/categraf/config.Version=$(VER)"

pack:
	env GOOS=linux GOARCH=amd64 go build -ldflags "-w -s -X flashcat.cloud/categraf/config.VERSION=$(VER)"
	env GOOS=windows GOARCH=amd64 go build -ldflags "-w -s -X flashcat.cloud/categraf/config.VERSION=$(VER)"
	rm -rf $(APP)-$(VER).tar.gz
	rm -rf $(APP)-$(VER).zip
	tar -zcvf $(APP)-$(VER).tar.gz conf $(APP)
	zip -r $(APP)-$(VER).zip conf $(APP).exe
