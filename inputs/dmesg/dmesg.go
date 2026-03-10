package dmesg

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "dmesg"

const (
	defaultBufSize = uint32(1 << 14) // 16KB by default
	levelMask      = uint64(1<<3 - 1)
)

const (
	OOM_Error                           = "Out of memory"
	NF_CONNTRACK_TABLE_FULL             = "nf_conntrack: table full"
	Drop_Packet                         = "dropping packet"
	Will_Reset_Adapter                  = "will reset adapter"
	Memory_Error                        = "memory error"
	Reset_Successful_For_SCSI           = "Reset successful for scsi"
	Call_Trace                          = "Call Trace"
	Segfault                            = "segfault"
	NIC_Link_Down                       = "NIC Link is Down"
	EXT4_Fs_Error                       = "EXT4-fs error"
	Medium_Error                        = "Medium Error"
	Package_Temperature_Above_Threshold = "Package temperature above threshold"
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

// 创建一个包含所有错误信息的切片，用于遍历
// 这里手动将常量和对应的码绑定，确保顺序一致
var errorList = map[string]int{
	OOM_Error:                           0,
	NF_CONNTRACK_TABLE_FULL:             0,
	Drop_Packet:                         0,
	Will_Reset_Adapter:                  0,
	Memory_Error:                        0,
	Reset_Successful_For_SCSI:           0,
	Call_Trace:                          0,
	Segfault:                            0,
	NIC_Link_Down:                       0,
	EXT4_Fs_Error:                       0,
	Medium_Error:                        0,
	Package_Temperature_Above_Threshold: 0,
}

type Instance struct {
	config.InstanceConfig

	ExternalKeywords []string `toml:"external_keywords"`

	conn syscall.RawConn
}

func (ins *Instance) Init() error {

	var err error

	f, err := os.OpenFile("/dev/kmsg", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		log.Println("Error opening /dev/kmsg:", err)
		return err
	}

	ins.conn, err = f.SyscallConn()
	if err != nil {
		log.Println("Error getting raw connection:", err)
		return err
	}

	for _, keyword := range ins.ExternalKeywords {
		errorList[keyword] = 0
	}

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
		log.Println("Error reading from /dev/kmsg:", err)
		slist.PushFront(types.NewSample(inputName, "up", 1, nil))
		return
	}

	for _, d := range msgs {
		for keyword := range errorList {
			if strings.Contains(d.Text, keyword) {
				errorList[keyword] += 1
			}
		}
	}
	for keyword, count := range errorList {
		slist.PushFront(types.NewSample(inputName, "hit_keyword", count, map[string]string{
			"keyword": keyword,
		}))
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
