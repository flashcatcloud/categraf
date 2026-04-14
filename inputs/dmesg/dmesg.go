//go:build linux
// +build linux

package dmesg

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

const inputName = "dmesg"

const (
	defaultBufSize = uint32(1 << 14) // 16KB by default
	levelMask      = uint64(1<<3 - 1)
)

const (
	OomError                         = "Out of memory"
	NfConntrackTableFull             = "nf_conntrack: table full"
	DropPacket                       = "dropping packet"
	WillResetAdapter                 = "will reset adapter"
	MemoryError                      = "memory error"
	ResetSuccessfulForScsi           = "Reset successful for scsi"
	CallTrace                        = "Call Trace"
	Segfault                         = "segfault"
	NicLinkDown                      = "NIC Link is Down"
	Ext4FsError                      = "EXT4-fs error"
	MediumError                      = "Medium Error"
	PackageTemperatureAboveThreshold = "Package temperature above threshold"
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

type Instance struct {
	config.InstanceConfig

	ExternalKeywords []string `toml:"external_keywords"`

	conn syscall.RawConn
	file *os.File

	errorList map[string]int
}

func (ins *Instance) Init() error {

	var err error

	f, err := os.OpenFile("/dev/kmsg", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		klog.ErrorS(err, "error opening /dev/kmsg")
		return err
	}

	ins.conn, err = f.SyscallConn()
	if err != nil {
		f.Close()
		klog.ErrorS(err, "error getting raw connection")
		return err
	}

	ins.errorList = map[string]int{
		OomError:                         0,
		NfConntrackTableFull:             0,
		DropPacket:                       0,
		WillResetAdapter:                 0,
		MemoryError:                      0,
		ResetSuccessfulForScsi:           0,
		CallTrace:                        0,
		Segfault:                         0,
		NicLinkDown:                      0,
		Ext4FsError:                      0,
		MediumError:                      0,
		PackageTemperatureAboveThreshold: 0,
	}

	for _, keyword := range ins.ExternalKeywords {
		ins.errorList[keyword] = 0
	}

	ins.file = f

	return nil
}

type Dmesg struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Dmesg{}
	})
}

func (d *Dmesg) Clone() inputs.Input {
	return &Dmesg{}
}

func (d *Dmesg) Name() string {
	return inputName
}

func (d *Dmesg) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(d.Instances))
	for i := 0; i < len(d.Instances); i++ {
		ret[i] = d.Instances[i]
	}
	return ret
}

func (ins *Instance) Gather(slist *types.SampleList) {

	msgs := make([]Msg, 0)

	var syscallError error = nil
	err := ins.conn.Read(func(fd uintptr) bool {
		for {
			buf := make([]byte, defaultBufSize)
			_, err := syscall.Read(int(fd), buf)
			if err != nil {
				syscallError = err
				// EINVAL means buf is not enough, data would be truncated, but still can continue.
				if !errors.Is(err, syscall.EINVAL) {
					return true
				}
			}

			msg := parseData(buf)
			if msg == nil {
				continue
			}
			msgs = append(msgs, *msg)
		}
	})

	// EAGAIN means no more data, should be treated as normal.
	if syscallError != nil && !errors.Is(syscallError, syscall.EAGAIN) {
		err = syscallError
	}

	if err != nil {
		klog.ErrorS(err, "error reading from /dev/kmsg")
		slist.PushFront(types.NewSample(inputName, "up", 0, nil))
		return
	}

	slist.PushFront(types.NewSample(inputName, "up", 1, nil))
	for _, d := range msgs {
		for keyword := range ins.errorList {
			if strings.Contains(d.Text, keyword) {
				ins.errorList[keyword]++
			}
		}
	}
	for keyword, count := range ins.errorList {
		slist.PushFront(types.NewSample(inputName, "hit_keyword", count, map[string]string{
			"keyword": keyword,
		}))
	}

}

func (ins *Instance) Cleanup() {
	if ins.file != nil {
		ins.file.Close()
	}
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
