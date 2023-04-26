package greenplum

import (
	"log"
	"os/exec"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "greenplum"

type Greenplum struct {
	config.PluginConfig
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Greenplum{}
	})
}

func (e *Greenplum) Clone() inputs.Input {
	return &Greenplum{}
}

func (e *Greenplum) Name() string {
	return inputName
}

func (ins *Greenplum) Gather(slist *types.SampleList) {
	var tags = map[string]string{}
	bin, err := exec.LookPath("gpstate")
	if err != nil {
		return
	}

	out, err := exec.Command(bin, "-m").Output()
	if err != nil {
		return
	}
	stringOut := string(out)
	const splitHeader string = "Mirror     Datadir                         Port    Status    Data Status"
	const lastHeader string = "gpadmin-[INFO]:---"
	stateValue := stringOut[(strings.Index(stringOut, splitHeader) + len(splitHeader)) : strings.LastIndex(stringOut, lastHeader)-47]
	stateValue = strings.TrimSpace(stateValue)
	gpstate := strings.Fields(stateValue)
	if len(gpstate)%7 != 0 {
		log.Println("E! failed to parse gpstate -m output: %v", gpstate)
		return
	}
	line := len(gpstate) / 7
	for i := 0; i < line; i++ {
		gptags := map[string]string{
			"Mirror":  gpstate[i*7+2],
			"Datadir": gpstate[i*7+3],
			"Port":    gpstate[i*7+4],
		}
		for tag, value := range tags {
			gptags[tag] = value
		}
		var status int32 = 0
		var dataStatus int32 = 0
		if gpstate[i*7+5] == "Passive" {
			status = 1
		}
		if gpstate[i*7+6] == "Synchronized" {
			dataStatus = 1
		}
		slist.PushSample(inputName, "Status", status, gptags)
		slist.PushSample(inputName, "Data_Status", dataStatus, gptags)
	}
}
