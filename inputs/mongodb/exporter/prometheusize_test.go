package exporter

import (
	"regexp"
	"strings"
	"testing"
)

var (
	refSpecialCharsRe        = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	refRepeatedUnderscoresRe = regexp.MustCompile(`__+`)
	refDollarRe              = regexp.MustCompile(`\_$`)
)

// regexpPrometheusize is the previous regexp-based implementation, kept as the reference.
func regexpPrometheusize(s string) string {
	for _, pair := range prefixes {
		if strings.HasPrefix(s, pair[0]+".") {
			s = pair[1] + strings.TrimPrefix(s, pair[0])
			break
		}
	}

	s = refSpecialCharsRe.ReplaceAllString(s, "_")
	s = refDollarRe.ReplaceAllString(s, "")
	s = refRepeatedUnderscoresRe.ReplaceAllString(s, "_")
	s = strings.TrimPrefix(s, "_")

	return exporterPrefix + s
}

var prometheusizeInputs = []string{
	"", "_", "__", "___", ".", "..", "a", "a_", "a__", "a___", "_a", "__a", "._a", "a._", "a_.", "a._.",
	"serverStatus", "serverStatus.", "serverStatus.wiredTiger", "serverStatus.wiredTiger.transaction.",
	"serverStatus.wiredTiger.cache.bytes currently in the cache",
	"serverStatus.wiredTiger.transaction.transaction checkpoint max time (msecs)",
	"serverStatusX.foo", "replSetGetStatus.members.optimeDate", "collstats.storageStats.indexSizes._id_",
	"systemMetrics.disks.nvme0n1.reads", "a-b/c d$e", "日本.語", "a\xffb", "\xff", "a\xff",
}

func TestPrometheusizeMatchesRegexp(t *testing.T) {
	for _, s := range prometheusizeInputs {
		if got, want := prometheusize(s), regexpPrometheusize(s); got != want {
			t.Errorf("prometheusize(%q) = %q, want %q", s, got, want)
		}
	}
}

func FuzzPrometheusizeMatchesRegexp(f *testing.F) {
	for _, s := range prometheusizeInputs {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := prometheusize(s), regexpPrometheusize(s); got != want {
			t.Errorf("prometheusize(%q) = %q, want %q", s, got, want)
		}
	})
}

func BenchmarkPrometheusize(b *testing.B) {
	s := "serverStatus.wiredTiger.cache.bytes currently in the cache"
	b.ReportAllocs()
	for b.Loop() {
		prometheusize(s)
	}
}
