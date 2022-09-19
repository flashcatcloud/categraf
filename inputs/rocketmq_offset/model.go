package rocketmq_offset

type TopicListData struct {
	TopicList  []string `json:"topicList"`
	BrokerAddr string   `json:"brokerAddr"`
}

type TopicList struct {
	Status int           `json:"status"`
	Data   TopicListData `json:"data"`
	ErrMsg string        `json:"errMsg"`
}

//(2).get consumerList by topic

type ConsumerListByTopic struct {
	Status int                   `json:"status"`
	ErrMsg string                `json:"errMsg"`
	Data   map[string]TopicGroup `json:"data"`
}

type TopicGroup struct {
	Topic             string              `json:"topic"`
	DiffTotal         int                 `json:"diffTotal"`
	LastTimestamp     int64               `json:"lastTimestamp"`
	QueueStatInfoList []QueueStatInfoList `json:"queueStatInfoList"`
}

type QueueStatInfoList struct {
	BrokerName     string `json:"brokerName"`
	QueueId        int    `json:"queueId"`
	BrokerOffset   int64  `json:"brokerOffset"`
	ConsumerOffset int64  `json:"consumerOffset"`
	ClientInfo     string `json:"clientInfo"`
	LastTimestamp  int64  `json:"lasttimestamp"`
}

//(3).mode for prometheus metrics

type MsgDiff struct {
	MsgDiffDetails               []*MsgDiffDetail                      `json:"msg_diff_details"`
	MsgDiffTopics                map[string]*MsgDiffTopic              `json:"msg_diff_topics"`
	MsgDiffConsumerGroups        map[string]*MsgDiffConsumerGroup      `json:"msg_diff_consumergroups"`
	MsgDiffTopics_ConsumerGroups map[string]*MsgDiffTopicConsumerGroup `json:"msg_diff_topics_consumergroups"`
	MsgDiffBrokers               map[string]*MsgDiffBroker             `json:"msg_diff_brokers"`
	MsgDiffQueues                map[string]*MsgDiffQueue              `json:"msg_diff_queues"`
	MsgDiffClientInfos           map[string]*MsgDiffClientInfo         `json:"msg_diff_clientinfos"`
}

type MsgDiffDetail struct {
	Broker            string `json:"broker"`
	QueueId           int    `json:"queueId"`
	ConsumerClientIP  string `json:"consumerClientIP"`
	ConsumerClientPID string `json:"consumerClientPID"`
	Diff              int    `json:"diff"`
	Topic             string `json:"topic"`
	ConsumerGroup     string `json:"consumerGroup"`
}

type MsgDiffTopic struct {
	Diff  int    `json:"diff"`
	Topic string `json:"topic"`
}

type MsgDiffConsumerGroup struct {
	Diff          int    `json:"diff"`
	ConsumerGroup string `json:"consumerGroup"`
}

type MsgDiffTopicConsumerGroup struct {
	Diff          int    `json:"diff"`
	Topic         string `json:"topic"`
	ConsumerGroup string `json:"consumerGroup"`
}

type MsgDiffBroker struct {
	Broker string `json:"broker"`
	Diff   int    `json:"diff"`
}

type MsgDiffQueue struct {
	Broker  string `json:"broker"`
	QueueId int    `json:"queueId"`
	Diff    int    `json:"diff"`
}

type MsgDiffClientInfo struct {
	ConsumerClientIP  string `json:"consumerClientIP"`
	ConsumerClientPID string `json:"consumerClientPID"`
	Diff              int    `json:"diff"`
}
