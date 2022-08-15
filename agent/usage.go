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
	url = "http://n9e.io/report"
)

func do() {
	hostname, err := os.Hostname()
	if err != nil {
		return
	}

	u := struct {
		Samples    float64
		Users      float64
		Hostname   string
		Version    string
		Maintainer string
	}{
		Samples:    1.0,
		Users:      1.0,
		Hostname:   hostname,
		Maintainer: "categraf",
		Version:    config.Version,
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
	go func() {
		for {
			select {
			case <-timer.C:
				do()
				timer.Reset(10 * time.Minute)
			}
		}
	}()
}
