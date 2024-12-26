package collector

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	crontabDir = new(string)
	// kingpin.Flag("collector.crontab.dir", "crontab监听目录").Default("/var/spool/cron/crontabs/").String()
	crontabMap = make(map[string]string)
)

type crontabCollector struct {
	cronNotify *prometheus.Desc
}

func init() {
	registerCollector("crontab", defaultDisabled, NewCrontabCollector)
}

func NewCrontabCollector() (Collector, error) {
	c := &crontabCollector{
		cronNotify: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "crontab"),
			"禁停crontab变化",
			[]string{"user"}, nil,
		),
	}
	fileList, _ := os.ReadDir(*crontabDir)

	for _, file := range fileList {
		if file.IsDir() {
			continue
		}
		if data, err := c.readFile(path.Join(*crontabDir, file.Name())); err != nil {
			panic(err.Error())
		} else {
			crontabMap[file.Name()] = fmt.Sprintf("%x", md5.Sum(data))
		}

	}
	return c, nil
}

func (c *crontabCollector) readFile(fileName string) ([]byte, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fd, _ := io.ReadAll(file)
	return fd, nil
}

func crontabCollectorInit(params map[string]string) {
	dir, ok := params["collector.crontab.dir"]
	if !ok {
		*crontabDir = "/var/spool/cron/crontabs/"
	} else {
		*crontabDir = dir
	}
}

func (c *crontabCollector) Update(ch chan<- prometheus.Metric) error {
	fileList, _ := os.ReadDir(*crontabDir)
	for _, file := range fileList {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		var v float64 = 0
		if data, ok := crontabMap[fileName]; !ok {
			v = 1
			filedata, _ := c.readFile(path.Join(*crontabDir, fileName))
			crontabMap[fileName] = fmt.Sprintf("%x", md5.Sum(filedata))

		} else {
			data1, err := c.readFile(path.Join(*crontabDir, fileName))
			if err != nil {
				v = 1
			}
			newMd5 := fmt.Sprintf("%x", md5.Sum(data1))
			if data != newMd5 {
				v = 1
				crontabMap[fileName] = newMd5
			}
		}
		ch <- prometheus.MustNewConstMetric(c.cronNotify, prometheus.GaugeValue, v, fileName)
	}
	return nil
}
