package rocketmq_offset

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/types"
)

const inputName = "rocketmq_offset"

const consoleSchema string = "http://"
const topicNameListPath string = "/topic/list.query"
const queryConsumerByTopicPath string = "/topic/queryConsumerByTopic.query?topic="

type RocketMQ struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &RocketMQ{}
	})
}

func (pt *RocketMQ) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(pt.Instances))
	for i := 0; i < len(pt.Instances); i++ {
		ret[i] = pt.Instances[i]
	}
	return ret
}

type Instance struct {
	config.InstanceConfig
	IgnoredTopics            []string `toml:"ignored_topics"`
	RocketMQConsoleIPAndPort string   `toml:"rocketmq_console_ip_port"`
}

func (ins *Instance) Gather(slist *types.SampleList) {
	//获取rocketmq集群中的topicNameList
	topicNameArray := GetTopicNameList(ins.RocketMQConsoleIPAndPort)
	if topicNameArray == nil {
		log.Println("E! fail to get topic,please check config!")
		return
	}

	//按照topic聚合msgDiff
	var diff_Topic_Map = make(map[string]*MsgDiffTopic)

	//按照consumerGroup聚合msgDiff
	//var diff_ConsumerGroup_Slice []model.MsgDiff_ConsumerGroup = []model.MsgDiff_ConsumerGroup{}
	var diff_ConsumerGroup_Map = make(map[string]*MsgDiffConsumerGroup)

	//按照topic, consumeGroup聚合msgDiff
	//var diff_Topic_ConsumerGroup_Slice []model.MsgDiff_Topics_ConsumerGroup = []model.MsgDiff_Topics_ConsumerGroup{}
	var diff_Topic_ConsumerGroup_Map = make(map[string]*MsgDiffTopicConsumerGroup)

	//按照broker聚合msgDiff
	//var diff_Broker_Slice []model.MsgDiff_Broker = []model.MsgDiff_Broker{}
	var diff_Broker_Map = make(map[string]*MsgDiffBroker)

	//按照clientInfo聚合msgDiff
	//var diff_Clientinfo_Slice []model.MsgDiff_ClientInfo = []model.MsgDiff_ClientInfo{}
	var diff_Clientinfo_Map = make(map[string]*MsgDiffClientInfo)

	//按照queue聚合msgDiff
	//var MsgDiff_Queue_Slice []model.MsgDiff_Queue = []model.MsgDiff_Queue{}
	var diff_Queue_Map = make(map[string]*MsgDiffQueue)

	for i := range topicNameArray {
		var topicName = topicNameArray[i]
		isContain := choice.Contains(topicName, ins.IgnoredTopics)
		if isContain {
			continue
		}

		var data *ConsumerListByTopic = GetConsumerListByTopic(ins.RocketMQConsoleIPAndPort, topicName)

		if data == nil {
			continue
		}

		topicConsumerGroups := data.Data

		for cgName, consumerInfo := range topicConsumerGroups {
			topic := consumerInfo.Topic

			//获取当前consumer信息及对应的rocketmq-queue的信息
			queueStatInfoList := consumerInfo.QueueStatInfoList

			for i := range queueStatInfoList {

				queue := queueStatInfoList[i]

				brokerName := queue.BrokerName
				queueId := queue.QueueId

				clientInfo := queue.ClientInfo
				consumerClientIP := ""
				consumerClientPID := ""
				if &clientInfo != nil {
					temp_array := strings.Split(clientInfo, "@")
					if temp_array != nil {
						if len(temp_array) == 1 {
							consumerClientIP = temp_array[0]
						} else if len(temp_array) == 2 {
							consumerClientIP = temp_array[0]
							consumerClientPID = temp_array[1]
						}
					}
				}

				diff := int(queue.BrokerOffset) - int(queue.ConsumerOffset)

				tags := map[string]string{
					"BrokerName":        brokerName,
					"QueueId":           fmt.Sprint(queueId),
					"ConsumerClientIP":  consumerClientIP,
					"ConsumerClientPID": consumerClientPID,
					"Topic":             topic,
					"ConsumerGroup":     cgName,
				}
				slist.PushSample(inputName, "diffDetail", diff, tags)

				//按照topic进行msgDiff聚合
				if _, ok := diff_Topic_Map[topic]; ok {
					//如果已经存在，计算diff
					diff_Topic_Map[topic].Diff = diff_Topic_Map[topic].Diff + diff
				} else {
					var diffTopic *MsgDiffTopic = new(MsgDiffTopic)

					diffTopic.Diff = diff
					diffTopic.Topic = topic

					diff_Topic_Map[topic] = diffTopic
				}

				//按照consumerGroup进行msgDiff聚合
				if _, ok := diff_ConsumerGroup_Map[cgName]; ok {
					diff_ConsumerGroup_Map[cgName].Diff = diff_ConsumerGroup_Map[cgName].Diff + diff
				} else {
					var diffConsumerGroup *MsgDiffConsumerGroup = new(MsgDiffConsumerGroup)

					diffConsumerGroup.ConsumerGroup = cgName
					diffConsumerGroup.Diff = diff

					diff_ConsumerGroup_Map[cgName] = diffConsumerGroup
				}

				//按照topic, consumerGroup进行msgDiff聚合
				topic_cgName := topic + ":" + cgName
				if _, ok := diff_Topic_ConsumerGroup_Map[topic_cgName]; ok {
					diff_Topic_ConsumerGroup_Map[topic_cgName].Diff = diff_Topic_ConsumerGroup_Map[topic_cgName].Diff + diff

				} else {
					var diff_topic_cg *MsgDiffTopicConsumerGroup = new(MsgDiffTopicConsumerGroup)

					diff_topic_cg.ConsumerGroup = cgName
					diff_topic_cg.Diff = diff
					diff_topic_cg.Topic = topic

					diff_Topic_ConsumerGroup_Map[topic_cgName] = diff_topic_cg

				}

				//按照broker进行msgDiff聚合
				if _, ok := diff_Broker_Map[brokerName]; ok {
					diff_Broker_Map[brokerName].Diff = diff_Broker_Map[brokerName].Diff + diff
				} else {
					var diff_Broker *MsgDiffBroker = new(MsgDiffBroker)

					diff_Broker.Broker = brokerName
					diff_Broker.Diff = diff

					diff_Broker_Map[brokerName] = diff_Broker
				}

				//按照queueId进行msgDiff聚合
				queuestr := brokerName + ":" + string(queueId)
				if _, ok := diff_Queue_Map[string(queueId)]; ok {
					diff_Queue_Map[queuestr].Diff = diff_Queue_Map[queuestr].Diff + diff
				} else {
					var diff_Queue *MsgDiffQueue = new(MsgDiffQueue)

					diff_Queue.Broker = brokerName
					diff_Queue.Diff = diff
					diff_Queue.QueueId = queueId

					diff_Queue_Map[queuestr] = diff_Queue
				}

				//按照clientInfo进行msgDiff聚合

				if _, ok := diff_Clientinfo_Map[clientInfo]; ok {
					diff_Clientinfo_Map[clientInfo].Diff = diff_Clientinfo_Map[clientInfo].Diff + diff
				} else {
					var diff_ClientInfo *MsgDiffClientInfo = new(MsgDiffClientInfo)

					diff_ClientInfo.ConsumerClientIP = consumerClientIP
					diff_ClientInfo.ConsumerClientPID = consumerClientPID
					diff_ClientInfo.Diff = diff

					diff_Clientinfo_Map[clientInfo] = diff_ClientInfo
				}

			}
		}

	}
	for topic, value := range diff_Topic_Map {
		tags := map[string]string{
			"Topic": topic,
		}
		slist.PushSample(inputName, "diffTopic", value.Diff, tags)
	}
	for ConsumerGroup, value := range diff_ConsumerGroup_Map {
		tags := map[string]string{
			"ConsumerGroup": ConsumerGroup,
		}
		slist.PushSample(inputName, "diffConsumerGroup", value.Diff, tags)
	}

	for topic_cgName, value := range diff_Topic_ConsumerGroup_Map {
		tags := map[string]string{
			"Topic":         strings.Split(topic_cgName, ":")[0],
			"ConsumerGroup": strings.Split(topic_cgName, ":")[1],
		}
		slist.PushSample(inputName, "diffTopicConsumerGroup", value.Diff, tags)
	}
	for broker, value := range diff_Broker_Map {
		tags := map[string]string{
			"Broker": broker,
		}
		slist.PushSample(inputName, "diffBroker", value.Diff, tags)
	}
	for queuestr, value := range diff_Queue_Map {
		tags := map[string]string{
			"Broker":  strings.Split(queuestr, ":")[0],
			"QueueId": strings.Split(queuestr, ":")[1],
		}
		slist.PushSample(inputName, "diffBrokerQueue", value.Diff, tags)
	}
	for _, value := range diff_Clientinfo_Map {
		tags := map[string]string{
			"ConsumerClientIP":  value.ConsumerClientIP,
			"ConsumerClientPID": value.ConsumerClientPID,
		}
		slist.PushSample(inputName, "diffClientInfo", value.Diff, tags)
	}

}

func GetTopicNameList(rocketmqConsoleIPAndPort string) []string {
	var url = consoleSchema + rocketmqConsoleIPAndPort + topicNameListPath
	var content = doRequest(url)

	var jsonData TopicList
	err := json.Unmarshal([]byte(content), &jsonData)

	if err != nil {
		log.Println("E! unable to decode topic name list", err)
		return nil
	}

	return jsonData.Data.TopicList
}

func GetConsumerListByTopic(rocketmqConsoleIPAndPort string, topicName string) *ConsumerListByTopic {
	var url = consoleSchema + rocketmqConsoleIPAndPort + queryConsumerByTopicPath + topicName
	var content = doRequest(url)

	var jsonData *ConsumerListByTopic
	err := json.Unmarshal([]byte(content), &jsonData)

	if err != nil {
		return nil
	}

	return jsonData
}

func doRequest(url string) []byte {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		log.Println("E! fail to read request data", err)
		return nil
	} else {
		return body
	}
}
