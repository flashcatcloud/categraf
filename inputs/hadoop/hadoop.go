package hadoop

import (
	"errors"
	"fmt"
	"github.com/emirpasic/gods/lists/singlylinkedlist"
	"log"
	"sync"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "hadoop"

type CommonConfig struct {
	UseSASL             bool   `default:"false" toml:"useSASL"`
	SaslUsername        string `toml:"saslUsername"`
	SaslDisablePAFXFast bool   `default:"true" toml:"saslDisablePAFXFast"`
	SaslMechanism       string `toml:"saslMechanism"`
	KerberosAuthType    string `toml:"kerberosAuthType"`
	KeyTabPath          string `toml:"keyTabPath"`
	KerberosConfigPath  string `toml:"kerberosConfigPath"`
	Realm               string `toml:"realm"`
}

type Hadoop struct {
	config.PluginConfig
	CommonConfig
	Components []ComponentOption `toml:"components"`
	components *singlylinkedlist.List
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Hadoop{}
	})
}

func (j *Hadoop) Clone() inputs.Input {
	return &Hadoop{}
}

func (j *Hadoop) Name() string {
	return inputName
}

func (ins *Hadoop) Init() error {
	if len(ins.Components) == 0 {
		return errors.New("no components configured")
	}

	ins.components = singlylinkedlist.New()

	for _, componentOption := range ins.Components {
		c := &Component{
			ComponentOption: componentOption,
		}
		if err := c.Initialize(ins.CommonConfig); err != nil {
			return fmt.Errorf("failed to initialize component %s: %v", componentOption.Name, err)
		}
		ins.components.Add(c)
	}
	return nil
}

func (ins *Hadoop) Gather(slist *types.SampleList) {
	if ins.components.Size() == 0 {
		return
	}

	var wg = sync.WaitGroup{}
	componentCollect := func(component *Component) {
		defer wg.Done()
		if !component.IsProcessExisted() {
			return
		}

		data, getDataErr := component.GetData(component.ComposeMetricUrl())
		if getDataErr != nil {
			log.Printf("E! Failed to get data from %s: %v", component.Name, getDataErr)
			return
		}

		res, fetchDataErr := component.FetchData(data)
		if fetchDataErr != nil {
			log.Printf("E! Failed to fetch data from %s: %v", component.Name, fetchDataErr)
			return
		}

		// Add metrics to slist
		for _, metricsData := range res {
			labels := metricsData.LabelPair
			labels["component"] = component.Name

			slist.PushSample(inputName,
				metricsData.Name,
				metricsData.Value,
				labels)
		}
	}

	for iter := ins.components.Iterator(); iter.Next(); {
		wg.Add(1)
		go componentCollect(iter.Value().(*Component))
	}
	wg.Wait()
}
