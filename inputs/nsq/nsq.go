package nsq

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "nsq"

type Topic struct {
	Name     string `json:"name"`
	Channels []struct {
		Depth int `json:"depth"`
	} `json:"channels"`
}

type ApiData struct {
	Topics  []string `json:"topics"`
	Message string   `json:"message"`
}

type Nsq struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Nsq{}
	})
}

func (pt *Nsq) Clone() inputs.Input {
	return &Nsq{}
}

func (pt *Nsq) Name() string {
	return inputName
}

func (pt *Nsq) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	URL string `toml:"url"`
}

func (ins *Instance) Init() error {
	if len(ins.URL) == 0 {
		return types.ErrInstancesEmpty
	}
	return nil
}

func (ins *Instance) GetTopicInfo() ([]string, error) {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", ins.URL, nil)
	if err != nil {
		return nil, err
	}
	//req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apidata ApiData
	if err := json.Unmarshal(body, &apidata); err != nil {
		return nil, err
	}

	return apidata.Topics, nil
}

func getQueuesInfo(URL, topicName string) (int, error) {
	client := http.Client{Timeout: 5 * time.Second}
	urlAll := fmt.Sprintf("%s/%s", URL, topicName)

	req, err := http.NewRequest("GET", urlAll, nil)
	if err != nil {
		return 0, err
	}
	//req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, err
	}

	channels, ok := data["channels"].([]interface{})
	if !ok || len(channels) == 0 {
		return 0, nil
	}

	channel, ok := channels[0].(map[string]interface{})
	if !ok {
		return 0, nil
	}

	depth, ok := channel["depth"].(float64)
	if !ok {
		return 0, nil
	}

	return int(depth), nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	topics, err := ins.GetTopicInfo()
	if err != nil {
		log.Println("Failed to obtain the topic list error:", err)
	} else {
		for _, topic := range topics {
			v, err := getQueuesInfo(ins.URL, topic)
			if err != nil {
				v = 0
				log.Println("Failed to obtain topic depth value error:", err)
			}
			fields := map[string]interface{}{
				"channel_depth": v,
			}
			tags := map[string]string{
				"topic_name": topic,
			}

			slist.PushSamples("nsq", fields, tags)
		}
	}
}
