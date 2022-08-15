package agent

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"flashcat.cloud/categraf/config"
)

const (
	url = "http://n9e.io/categraf"
)

func do() {
	hostname, err := os.Hostname()
	if err != nil {
		return
	}

	u := struct {
		Hostname string
		Version  string
		Job      string
		User     string
	}{
		Hostname: hostname,
		Version:  config.Version,
		Job:      "categraf",
		User:     "",
	}
	body, err := json.Marshal(u)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return
	}

	cli := http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := cli.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		return
	}

	_, err = ioutil.ReadAll(resp.Body)
	return
}

func Report() {
	if config.Config.DisableUsageReport {
		return
	}
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			do()
			timer.Reset(10 * time.Minute)
		}
	}
}
