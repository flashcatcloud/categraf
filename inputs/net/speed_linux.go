//go:build linux

package net

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func Speed(iface string) (int64, error) {
	speedFile := fmt.Sprintf("/sys/class/net/%s/speed", iface)
	var speed int64
	if content, err := os.ReadFile(speedFile); err == nil {
		speed, err = strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
		if err != nil {
			return 0, err
		}
	} else {
		cmd := exec.Command("ethtool", iface)
		if content, err := cmd.CombinedOutput(); err == nil {
			var speedStr string

			contentReader := bufio.NewReader(bytes.NewBuffer(content))
			for {
				line, err := readLine(contentReader)

				if err == io.EOF {
					err = nil
					break
				}

				if err != nil {
					break
				}

				line = bytes.Trim(line, "\t")

				if bytes.HasPrefix(line, []byte("Speed:")) && bytes.HasSuffix(line, []byte("Mb/s")) {
					speedStr = string(line[7 : len(line)-4])
					break
				}
			}

			speed, err = strconv.ParseInt(strings.TrimSpace(speedStr), 10, 64)
			if err != nil {
				return 0, err
			}
		}
	}
	return speed, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, isPrefix, err := r.ReadLine()
	for isPrefix && err == nil {
		var bs []byte
		bs, isPrefix, err = r.ReadLine()
		line = append(line, bs...)
	}

	return line, err
}
