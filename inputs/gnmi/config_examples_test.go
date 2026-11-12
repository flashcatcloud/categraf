package gnmi

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestDocumentedHealthConfigurations(t *testing.T) {
	blocks := regexp.MustCompile("(?s)```toml\\s*\\n(.*?)```")
	for _, doc := range []struct{ file, heading string }{{"README.md", "## Complete configuration examples"}, {"README_CN.md", "## 完整配置案例"}} {
		text, err := os.ReadFile(doc.file)
		if err != nil {
			t.Fatal(err)
		}
		sections := strings.SplitN(string(text), doc.heading, 2)
		if len(sections) != 2 {
			t.Fatalf("missing examples in %s", doc.file)
		}
		examples := blocks.FindAllStringSubmatch(sections[1], -1)
		if len(examples) != 4 {
			t.Fatalf("examples=%d in %s, want 4", len(examples), doc.file)
		}
		for index, example := range examples {
			var plugin GNMI
			metadata, err := toml.Decode(example[1], &plugin)
			if err != nil {
				t.Fatalf("%s example %d: %v", doc.file, index+1, err)
			}
			if len(metadata.Undecoded()) != 0 {
				t.Fatalf("unknown options in %s example %d: %v", doc.file, index+1, metadata.Undecoded())
			}
			if err := plugin.Init(); err != nil {
				t.Fatal(err)
			}
			for _, instance := range plugin.Instances {
				if err := instance.initHealth(); err != nil {
					t.Fatal(err)
				}
				if err := instance.initICMP(); err != nil {
					t.Fatal(err)
				}
				instance.SetInitialized()
			}
			if index == 3 && (!plugin.EnableEndpointUp || len(plugin.Instances) != 2 || len(plugin.newICMPMonitor().keys) != 1) {
				t.Fatalf("aggregation/shared-probe example does not match its documented behavior in %s", doc.file)
			}
		}
	}
}
