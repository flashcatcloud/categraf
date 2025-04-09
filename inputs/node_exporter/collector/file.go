package collector

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	fileList = new(string)
	// kingpin.Flag("collector.filenotify.list", "监听文件列表，多个文件以 , 隔开").Default("/etc/passwd,/etc/shadow").String()
	fileMap = make(map[string]string)
)

type fileListCollector struct {
	fileNotify *prometheus.Desc
}

func init() {
	registerCollector("filenotify", defaultDisabled, NewFileNotifyCollector)
}

func NewFileNotifyCollector() (Collector, error) {
	f := &fileListCollector{
		fileNotify: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "filenotify"),
			"监听对应文件变化",
			[]string{"file_name"}, nil,
		),
	}
	for _, fileName := range strings.Split(*fileList, ",") {
		if len(strings.TrimSpace(fileName)) == 0 {
			continue
		}
		if data, err := f.readFile(fileName); err != nil {
			panic(err.Error())
		} else {
			fileMap[fileName] = fmt.Sprintf("%x", md5.Sum(data))
		}

	}
	return f, nil
}

func (f *fileListCollector) readFile(fileName string) ([]byte, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fd, _ := io.ReadAll(file)
	return fd, nil
}

func fileCollectorInit(params map[string]string) {
	files, ok := params["collector.filenotify.list"]
	if !ok {
		if runtime.GOOS == "linux" {
			*fileList = "/etc/passwd,/etc/shadow"
		} else {
			*fileList = ""
		}
	} else {
		*fileList = files
	}
}

func (f *fileListCollector) Update(ch chan<- prometheus.Metric) error {
	for fileName := range fileMap {
		var v float64 = 0
		data, err := f.readFile(fileName)
		if err != nil {
			v = 1
		}
		newMd5 := fmt.Sprintf("%x", md5.Sum(data))
		if fileMap[fileName] != newMd5 {
			v = 1
			fileMap[fileName] = newMd5
		}
		ch <- prometheus.MustNewConstMetric(f.fileNotify, prometheus.GaugeValue, v, fileName)
	}

	return nil
}
