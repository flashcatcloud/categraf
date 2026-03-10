package dmesg

import (
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "dmesg"

const (
	GetError1  = "Out of memory"
	GetError2  = "nf_conntrack: table full"
	GetError3  = "dropping packet"
	GetError4  = "will reset adapter"
	GetError5  = "memory error"
	GetError6  = "Reset successful for scsi"
	GetError7  = "Call Trace"
	GetError8  = "segfault"
	GetError9  = "NIC Link is Down"
	GetError10 = "EXT4-fs error"
	GetError11 = "Medium Error"
	GetError12 = "Package temperature above threshold"
)

const (
	_         = iota // 跳过 0 和 1，让 iota 从 2 开始计数
	ErrCode1         // 2
	ErrCode2         // 3
	ErrCode3         // 4
	ErrCode4         // 5
	ErrCode5         // 6
	ErrCode6         // 7
	ErrCode7         // 8
	ErrCode8         // 9
	ErrCode9         // 10
	ErrCode10        // 11
	ErrCode11        // 12
	ErrCode12        // 13
)

type ErrorItem struct {
	Code    int
	Message string
}

// 创建一个包含所有错误信息的切片，用于遍历
// 这里手动将常量和对应的码绑定，确保顺序一致
var errorList = []ErrorItem{
	{Code: ErrCode1, Message: GetError1},
	{Code: ErrCode2, Message: GetError2},
	{Code: ErrCode3, Message: GetError3},
	{Code: ErrCode4, Message: GetError4},
	{Code: ErrCode5, Message: GetError5},
	{Code: ErrCode6, Message: GetError6},
	{Code: ErrCode7, Message: GetError7},
	{Code: ErrCode8, Message: GetError8},
	{Code: ErrCode9, Message: GetError9},
	{Code: ErrCode10, Message: GetError10},
	{Code: ErrCode11, Message: GetError11},
	{Code: ErrCode12, Message: GetError12},
}

type Instance struct {
	config.InstanceConfig

	ExternalKeywords []string `toml:"external_keywords"`
}

func (ins *Instance) Init() error {
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

	dmesgs, err := Dmesgs()

	if err != nil {
		slist.PushFront(types.NewSample(inputName, "up", 0, nil))
		return
	}

	for _, d := range dmesgs {
		for _, e := range errorList {
			if strings.Contains(d.Text, e.Message) {
				slist.PushFront(types.NewSample(inputName, "dmesg_error", 1, map[string]string{
					"keyword": e.Message,
				}))
			}
		}

		for _, k := range ins.ExternalKeywords {
			if strings.Contains(d.Text, k) {
				slist.PushFront(types.NewSample(inputName, "dmesg_external_keywords", 1, map[string]string{
					"keyword": k,
				}))
			}
		}
	}

}
