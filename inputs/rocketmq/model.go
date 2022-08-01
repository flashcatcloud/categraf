package rocketmq

type TopicList_Data struct {
	TopicList  []string `json:"topicList"`
	BrokerAddr string   `json:"brokerAddr"`
}

type TopicList struct {
	Status int            `json:"status"`
	Data   TopicList_Data `json:"data"`
	ErrMsg string         `json:"errMsg"`
}

//(2).get consumerList by topic

type ConsumerList_By_Topic struct {
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
	MsgDiff_Details               []*MsgDiff_Detail                       `json:"msg_diff_details"`
	MsgDiff_Topics                map[string]*MsgDiff_Topic               `json:"msg_diff_topics"`
	MsgDiff_ConsumerGroups        map[string]*MsgDiff_ConsumerGroup       `json:"msg_diff_consumergroups"`
	MsgDiff_Topics_ConsumerGroups map[string]*MsgDiff_Topic_ConsumerGroup `json:"msg_diff_topics_consumergroups"`
	MsgDiff_Brokers               map[string]*MsgDiff_Broker              `json:"msg_diff_brokers"`
	MsgDiff_Queues                map[string]*MsgDiff_Queue               `json:"msg_diff_queues"`
	MsgDiff_ClientInfos           map[string]*MsgDiff_ClientInfo          `json:"msg_diff_clientinfos"`
}

type MsgDiff_Detail struct {
	Broker            string `json:"broker"`
	QueueId           int    `json:"queueId"`
	ConsumerClientIP  string `json:"consumerClientIP"`
	ConsumerClientPID string `json:"consumerClientPID"`
	Diff              int    `json:"diff"`
	Topic             string `json:"topic"`
	ConsumerGroup     string `json:"consumerGroup"`
}

type MsgDiff_Topic struct {
	Diff  int    `json:"diff"`
	Topic string `json:"topic"`
}

type MsgDiff_ConsumerGroup struct {
	Diff          int    `json:"diff"`
	ConsumerGroup string `json:"consumerGroup"`
}

type MsgDiff_Topic_ConsumerGroup struct {
	Diff          int    `json:"diff"`
	Topic         string `json:"topic"`
	ConsumerGroup string `json:"consumerGroup"`
}

type MsgDiff_Broker struct {
	Broker string `json:"broker"`
	Diff   int    `json:"diff"`
}

type MsgDiff_Queue struct {
	Broker  string `json:"broker"`
	QueueId int    `json:"queueId"`
	Diff    int    `json:"diff"`
}

type MsgDiff_ClientInfo struct {
	ConsumerClientIP  string `json:"consumerClientIP"`
	ConsumerClientPID string `json:"consumerClientPID"`
	Diff              int    `json:"diff"`
}
