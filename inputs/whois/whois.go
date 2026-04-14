package whois

import (
	"sync"
	"time"

	"github.com/araddon/dateparse"
	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

const inputName = "whois"

type Whois struct {
	config.PluginConfig
	Mappings  map[string]map[string]string `toml:"mappings"`
	Instances []*Instance                  `toml:"instances"`
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
	inputLabels := wh.GetLabels()
	for i := 0; i < len(wh.Instances); i++ {
		if len(wh.Instances[i].Labels) == 0 {
			wh.Instances[i].Labels = inputLabels
		} else {
			for k, v := range inputLabels {
				if _, has := wh.Instances[i].Labels[k]; !has {
					wh.Instances[i].Labels[k] = v
				}
			}
		}
		if len(wh.Instances[i].Mappings) == 0 {
			wh.Instances[i].Mappings = wh.Mappings
		} else {
			m := make(map[string]map[string]string)
			for k, v := range wh.Mappings {
				m[k] = v
			}
			for k, v := range wh.Instances[i].Mappings {
				m[k] = v
			}
			wh.Instances[i].Mappings = m
		}
		ret[i] = wh.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	Domain     string        `toml:"domain"`
	Domains    []string      `toml:"domains"`
	Timeout    int           `toml:"timeout"`
	Server     string        `toml:"server"`
	Concurrent int           `toml:"concurrent"`
	client     *whois.Client `toml:"-"`
	domains    []string      `toml:"-"`

	Mappings map[string]map[string]string `toml:"mappings"`
}

func (ins *Instance) Empty() bool {
	if len(ins.Domain) > 0 || len(ins.Domains) > 0 {
		return false
	}

	return true
}
func (ins *Instance) Init() error {
	if ins.Empty() {
		return types.ErrInstancesEmpty
	}

	ins.client = whois.NewClient()
	if ins.Timeout != 0 {
		ins.client.SetTimeout(time.Duration(ins.Timeout) * time.Second)
	}

	ins.domains = make([]string, 0)
	if ins.Domain != "" {
		ins.domains = append(ins.domains, ins.Domain)
	}
	if len(ins.Domains) > 0 {
		ins.domains = append(ins.domains, ins.Domains...)
	}
	if ins.Concurrent <= 0 {
		ins.Concurrent = 1 // 默认为1，顺序执行
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	if len(ins.domains) == 0 {
		return
	}

	semaphore := make(chan struct{}, ins.Concurrent)
	var wg sync.WaitGroup

	for _, domain := range ins.domains {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量，控制并发数

		go func(domain string) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			ins.queryDomain(domain, slist)
		}(domain)
	}

	wg.Wait()
}

func (ins *Instance) queryDomain(domain string, slist *types.SampleList) {
	var result string
	var err error
	maxRetries := 3

	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			sleepTime := time.Duration(retry) * 3 * time.Second
			time.Sleep(sleepTime)
			klog.Warningf("retrying whois query: attempt=%d max_retries=%d domain=%s", retry+1, maxRetries, domain)
		}

		result, err = ins.client.Whois(domain, ins.Server)
		if err == nil {
			break
		}

		klog.Warningf("whois query attempt failed: domain=%s attempt=%d err=%v", domain, retry+1, err)
	}

	if err != nil {
		klog.ErrorS(err, "failed to query domain information", "domain", domain)
		return
	}

	// 使用 whois-parser 解析结果
	parsedResult, err := whoisparser.Parse(result)
	if err != nil {
		klog.ErrorS(err, "failed to parse whois result", "domain", domain)
		return
	}

	// 输出解析后的信息
	if parsedResult.Domain.CreatedDate != "" || parsedResult.Domain.UpdatedDate != "" || parsedResult.Domain.ExpirationDate != "" {
		var CreatedDate, UpdatedDate, ExpirationDate int64
		fields := make(map[string]interface{})
		if parsedResult.Domain.CreatedDate != "" {
			CreatedDate, err = ParseTimeToUTCTimestamp(parsedResult.Domain.CreatedDate)
			if err != nil {
				klog.ErrorS(err, "failed to parse domain creation time", "domain", domain, "time", parsedResult.Domain.CreatedDate)
				return
			}
			fields["domain_createddate"] = CreatedDate
		} else {
			klog.ErrorS(nil, "domain creation time is empty", "domain", domain)
			return
		}

		// 有些域名不会返回UpdatedDate
		if parsedResult.Domain.UpdatedDate != "" {
			UpdatedDate, err = ParseTimeToUTCTimestamp(parsedResult.Domain.UpdatedDate)
			if err != nil {
				klog.ErrorS(err, "failed to parse domain update time", "domain", domain, "time", parsedResult.Domain.UpdatedDate)
			}
			fields["domain_updateddate"] = UpdatedDate
		} else {
			klog.Warningf("domain update time is empty: domain=%s", domain)
		}

		if parsedResult.Domain.ExpirationDate != "" {
			ExpirationDate, err = ParseTimeToUTCTimestamp(parsedResult.Domain.ExpirationDate)
			if err != nil {
				klog.ErrorS(err, "failed to parse domain expiration time", "domain", domain, "time", parsedResult.Domain.ExpirationDate)
				return
			}
			fields["domain_expirationdate"] = ExpirationDate
		} else {
			klog.ErrorS(nil, "domain expiration time is empty", "domain", domain)
			return
		}

		tags := map[string]string{
			"domain": domain,
		}
		if ls, ok := ins.Mappings[domain]; ok {
			for k, v := range ls {
				if _, exist := tags[k]; !exist {
					tags[k] = v
				}
			}
		}

		slist.PushSamples(inputName, fields, tags)

	} else {
		klog.ErrorS(nil, "domain creation, update, and expiration times are all empty", "domain", domain)
		return
	}

}

// ParseTimeToUTCTimestamp 将时间字符串解析为 UTC 时间戳
func ParseTimeToUTCTimestamp(timeStr string) (int64, error) {
	// 解析时间字符串
	parsedTime, err := dateparse.ParseAny(timeStr)
	if err != nil {
		return 0, err
	}

	// 将时间转换为 UTC 时间戳
	utcTimestamp := parsedTime.UTC().Unix()

	return utcTimestamp, nil
}
