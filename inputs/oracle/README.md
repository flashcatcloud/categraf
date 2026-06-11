# oracle

该采集插件的原理，就是连上 oracle 实例，执行各种 sql 获取监控数据，oracle 是一个非常完备的老牌的数据库，所有的监控数据都在自己的库里存储，有相关视图供用户查询。

配置文件在 `conf/input.oracle` 目录下，oracle.toml 是配置连接地址，metric.toml 是配置查询监控数据的 sql，每个配置段的含义：

- mesurement: 自定义的一个指标前缀
- request: sql 语句
- label_fields: sql 查到的内容，会有多列，哪些列作为时序数据的 label
- metric_fields: sql 查到的内容，会有多列，哪些列作为时序数据的值
- field_to_append: 是否要把某列的内容附到指标名称里
- timeout: sql 执行的超时时间

有些字段可以为空，如果 mesurement、metric_fields、field_to_append 三个字段都配置了，会把这 3 部分拼成 metric 的最终名字，参考下面的代码：

```go
func (o *Oracle) parseRow(row map[string]string, metricConf MetricConfig, slist *types.SampleList, tags map[string]string) error {
	labels := make(map[string]string)
	for k, v := range tags {
		labels[k] = v
	}

	for _, label := range metricConf.LabelFields {
		labelValue, has := row[label]
		if has {
			labels[label] = strings.Replace(labelValue, " ", "_", -1)
		}
	}

	for _, column := range metricConf.MetricFields {
		value, err := conv.ToFloat64(row[column])
		if err != nil {
			log.Println("E! failed to convert field:", column, "value:", value, "error:", err)
			return err
		}

		if metricConf.FieldToAppend == "" {
			slist.PushFront(types.NewSample(metricConf.Mesurement+"_"+column, value, labels))
		} else {
			suffix := cleanName(row[metricConf.FieldToAppend])
			slist.PushFront(types.NewSample(metricConf.Mesurement+"_"+suffix+"_"+column, value, labels))
		}
	}

	return nil
}
```

## instantclient

oracle 采集插件需要依赖 [instantclient](https://www.oracle.com/database/technologies/instant-client/downloads.html) ，这是 Oracle 官方提供的lib库，启动 Categraf 之前，要导出 LD_LIBRARY_PATH 环境变量，举例：

```shell
export LD_LIBRARY_PATH=/opt/oracle/instantclient_21_5
cd /opt/categraf
nohup ./categraf &> stdout.log &
```

## 监控大盘

本 README 文件的同级目录下，提供了 dashboard.json 就是 Oracle 的监控大盘，可以导入夜莺使用。

## 更新 2022-06-24

从 v0.1.8 版本开始，每个 instances 实例下面也可以配置 SQL 了，这些 SQL 只生效到对应的实例，相当于：所有Oracle都要采集的 SQL 配置到全局的 metrics.toml，某个实例特殊的配置则配置在 instances 下面，配置文件中给了一个例子，`[[instances.metrics]]` 配置段。