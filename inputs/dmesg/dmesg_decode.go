// Package dmesg provides interfaces to get log messages from linux kernel ring buffer like
// cmd util 'dmesg' by reading data from /dev/kmsg.
package dmesg

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"syscall"
)

const (
	defaultBufSize = uint32(1 << 14) // 16KB by default
	levelMask      = uint64(1<<3 - 1)
)

type Msg struct {
	Level      uint64            // SYSLOG lvel
	Facility   uint64            // SYSLOG facility
	Seq        uint64            // Message sequence number
	TsUsec     int64             // Timestamp in microsecond
	Caller     string            // Message caller
	IsFragment bool              // This message is a fragment of an early message which is not a fragment
	Text       string            // Log text
	DeviceInfo map[string]string // Device info
}

type dmesg struct {
	raw [][]byte
	msg []Msg
}

func parseData(data []byte) *Msg {
	msg := Msg{}

	dataLen := len(data)
	prefixEnd := bytes.IndexByte(data, ';')
	if prefixEnd == -1 {
		return nil
	}

	for index, prefix := range bytes.Split(data[:prefixEnd], []byte(",")) {
		switch index {
		case 0:
			val, _ := strconv.ParseUint(string(prefix), 10, 64)
			msg.Level = val & levelMask
			msg.Facility = val & (^levelMask)
		case 1:
			val, _ := strconv.ParseUint(string(prefix), 10, 64)
			msg.Seq = val
		case 2:
			val, _ := strconv.ParseInt(string(prefix), 10, 64)
			msg.TsUsec = val
		case 3:
			msg.IsFragment = prefix[0] != '-'
		case 4:
			msg.Caller = string(prefix)
		}
	}

	textEnd := bytes.IndexByte(data, '\n')
	if textEnd == -1 || textEnd <= prefixEnd {
		return nil
	}

	msg.Text = string(data[prefixEnd+1 : textEnd])
	if textEnd == dataLen-1 {
		return nil
	}

	msg.DeviceInfo = make(map[string]string, 2)
	deviceInfo := bytes.Split(data[textEnd+1:dataLen-1], []byte("\n"))
	for _, info := range deviceInfo {
		if info[0] != ' ' {
			continue
		}

		kv := bytes.Split(info, []byte("="))
		if len(kv) != 2 {
			continue
		}

		msg.DeviceInfo[string(kv[0])] = string(kv[1])
	}

	return &msg
}

func fetch(bufSize uint32, fetchRaw bool) (dmesg, error) {
	d := dmesg{}
	file, err := os.OpenFile("/dev/kmsg", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return d, err
	}
	defer file.Close()

	var conn syscall.RawConn
	conn, err = file.SyscallConn()
	if err != nil {
		return d, err
	}

	if fetchRaw {
		d.raw = make([][]byte, 0)
	} else {
		d.msg = make([]Msg, 0)
	}

	var syscallError error = nil
	err = conn.Read(func(fd uintptr) bool {
		for {
			buf := make([]byte, bufSize)
			_, err := syscall.Read(int(fd), buf)
			if err != nil {
				syscallError = err
				// EINVAL means buf is not enough, data would be truncated, but still can continue.
				if !errors.Is(err, syscall.EINVAL) {
					return true
				}
			}

			if fetchRaw {
				d.raw = append(d.raw, buf)
			} else {
				msg := parseData(buf)
				if msg == nil {
					continue
				}
				d.msg = append(d.msg, *msg)
			}
		}
	})

	// EAGAIN means no more data, should be treated as normal.
	if syscallError != nil && !errors.Is(syscallError, syscall.EAGAIN) {
		err = syscallError
	}

	return d, err
}

// DmesgWithBufSize gets all messages from kernel ring buffer with specific buf size for each message.
// It returns serialized message structure and the error while getting messages.
func DmesgWithBufSize(bufSize uint32) ([]Msg, error) {
	d, err := fetch(bufSize, false)

	return d.msg, err
}

// RawDmesgWithBufSize gets all messages from kernel ring buffer with specific buf size for each message.
// It returns native message from kernel without parsing and the error while getting messages.
func RawDmesgWithBufSize(bufSize uint32) ([][]byte, error) {
	d, err := fetch(bufSize, true)

	return d.raw, err
}

// Dmesgs gets all messages from kernel ring buffer with default buf size 16KB for each message.
// It returns serialized message structure and the error while getting messages.
// The error syscall.EINVAL means the buf size is not enough, consider to use
// DmesgWithBufSize instead.
func Dmesgs() ([]Msg, error) {
	return DmesgWithBufSize(defaultBufSize)
}

// RawDmesg gets all messages from kernel ring buffer with default buf size 16KB for each message.
// It returns native message from kernel without parsing and the error while getting messages.
// The error syscall.EINVAL means the buf size is not enough, consider to use
// RawDmesgWithBufSize instead.
func RawDmesg() ([][]byte, error) {
	return RawDmesgWithBufSize(defaultBufSize)
}
