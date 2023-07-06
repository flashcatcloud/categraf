//go:build linux
// +build linux

package netstat

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	parser = regexp.MustCompile(`(?i)` +
		`\d+:\s+` + // sl
		`([a-f0-9]{8,32}):([a-f0-9]{4})\s+` + // local_address
		`([a-f0-9]{8,32}):([a-f0-9]{4})\s+` + // rem_address
		`([a-f0-9]{2})\s+` + // st
		`([a-f0-9]{8}):([a-f0-9]{8})\s+` + // tx_queue rx_queue
		`[a-f0-9]{2}:[a-f0-9]{8}\s+` + // tr tm->when
		`[a-f0-9]{8}\s+` + // retrnsmt
		`(\d+)\s+` + // uid
		`\d+\s+` + // timeout
		`(\d+)\s+` + // inode
		`.+`) // stuff we don't care about
)

const (
	defaultTrimSet = "\r\n\t "
)

// Trim remove trailing spaces from a string.
func Trim(s string) string {
	return strings.Trim(s, defaultTrimSet)
}

func decToInt(n string) int {
	d, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		log.Printf("Error while parsing %s to int: %s", n, err)
	}
	return int(d)
}

func hexToInt(h string) uint {
	d, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		log.Printf("Error while parsing %s to int: %s", h, err)
	}
	return uint(d)
}

func hexToInt2(h string) (uint, uint) {
	if len(h) > 16 {
		d, err := strconv.ParseUint(h[:16], 16, 64)
		if err != nil {
			log.Printf("Error while parsing %s to int: %s", h[16:], err)
		}
		d2, err := strconv.ParseUint(h[16:], 16, 64)
		if err != nil {
			log.Printf("Error while parsing %s to int: %s", h[16:], err)
		}
		return uint(d), uint(d2)
	}

	d, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		log.Printf("Error while parsing %s to int: %s", h[16:], err)
	}
	return uint(d), 0
}

func hexToIP(h string) net.IP {
	n, m := hexToInt2(h)
	var ip net.IP
	if m != 0 {
		ip = make(net.IP, 16)
		// TODO: Check if this depends on machine endianness?
		binary.LittleEndian.PutUint32(ip, uint32(n>>32))
		binary.LittleEndian.PutUint32(ip[4:], uint32(n))
		binary.LittleEndian.PutUint32(ip[8:], uint32(m>>32))
		binary.LittleEndian.PutUint32(ip[12:], uint32(m))
	} else {
		ip = make(net.IP, 4)
		binary.LittleEndian.PutUint32(ip, uint32(n))
	}
	return ip
}

// Parse scans and retrieves the opened connections, from /proc/net/ files
func Parse(proto string) ([]Entry, error) {
	filename := fmt.Sprintf("/proc/net/%s", proto)
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	entries := make([]Entry, 0)
	scanner := bufio.NewScanner(fd)
	for lineno := 0; scanner.Scan(); lineno++ {
		// 跳过列名
		if lineno == 0 {
			continue
		}

		line := Trim(scanner.Text())
		m := parser.FindStringSubmatch(line)
		if m == nil {
			log.Printf("Could not parse netstat line from %s: %s", filename, line)
			continue
		}
		//只统计状态为TCP_ESTABLISHED
		if m[5] == "01" {
			entries = append(entries, NewEntry(
				proto,
				hexToIP(m[1]),
				hexToInt(m[2]),
				hexToIP(m[3]),
				hexToInt(m[4]),
				hexToInt(m[6]), // tx_queue发送
				hexToInt(m[7]), // rx_queue接收
				decToInt(m[8]), //uid
				decToInt(m[9]), //inode
			))
		}
	}

	return entries, nil
}
