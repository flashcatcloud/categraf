package whois

import (
	"log"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "whois"

type Whois struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Whois{}
	})
}

func (wh *Whois) Clone() inputs.Input {
	return &Whois{}
}

func (wh *Whois) Name() string {
	return inputName
}

func (wh *Whois) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(wh.Instances))
	for i := 0; i < len(wh.Instances); i++ {
		ret[i] = wh.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	Domain string `toml:"domain"`
}

func (ins *Instance) Empty() bool {
	if len(ins.Domain) > 0 {
		return false
	}

	return true
}
func (ins *Instance) Init() error {
	if ins.Empty() {
		return types.ErrInstancesEmpty
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	// 执行 Whois 查询
	result, err := whois.Whois(ins.Domain)
	if err != nil {
		log.Println("E! query", ins.Domain, "domain information failed:", err)
		return
	}

	// 使用 whois-parser 解析结果
	parsedResult, err := whoisparser.Parse(result)
	if err != nil {
		log.Println("E! parse", ins.Domain, "domain whois result failure:", err)
		return
	}

	// 输出解析后的信息
	if parsedResult.Domain.CreatedDate != "" && parsedResult.Domain.ExpirationDate != "" && parsedResult.Domain.ExpirationDate != "" {
		CreatedDate, err := ParseTimeToUTCTimestamp(parsedResult.Domain.CreatedDate)
		if err != nil {
			log.Println("E! parsing creation time:", parsedResult.Domain.CreatedDate, "time string failure:", err)
			return
		}
		UpdatedDate, err := ParseTimeToUTCTimestamp(parsedResult.Domain.UpdatedDate)
		if err != nil {
			log.Println("E! parsing update time:", parsedResult.Domain.UpdatedDate, "time string failure:", err)
			return
		}
		ExpirationDate, err := ParseTimeToUTCTimestamp(parsedResult.Domain.ExpirationDate)
		if err != nil {
			log.Println("E! parsing expiration time:", parsedResult.Domain.ExpirationDate, "time string failure:", err)
			return
		}

		fields := map[string]interface{}{
			"domain_createddate":    CreatedDate,
			"domain_updateddate":    UpdatedDate,
			"domain_expirationdate": ExpirationDate,
		}
		tags := map[string]string{
			"domain": ins.Domain,
		}

		slist.PushSamples(inputName, fields, tags)

	} else {
		log.Println("E! creation or expiration time is null")
		return
	}

}

// ParseTimeToUTCTimestamp 将时间字符串解析为 UTC 时间戳
func ParseTimeToUTCTimestamp(timeStr string) (int64, error) {
	// 解析时间字符串
	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return 0, err
	}

	// 将时间转换为 UTC 时间戳
	utcTimestamp := parsedTime.UTC().Unix()

	return utcTimestamp, nil
}
