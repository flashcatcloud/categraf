package inputs

import (
	"testing"

	"flashcat.cloud/categraf/pkg/cfg"
)

func testConfigWithSum(sum string) cfg.ConfigWithFormat {
	c := cfg.ConfigWithFormat{
		Config: "test",
		Format: cfg.TomlFormat,
	}
	c.SetCheckSum(sum)
	return c
}

func TestInnerCacheGetReturnsCopy(t *testing.T) {
	cache := newInnerCache()
	cache.put("cpu", testConfigWithSum("sum-a"))

	got, ok := cache.get("cpu")
	if !ok {
		t.Fatal("expected cache entry")
	}

	got["sum-b"] = testConfigWithSum("sum-b")
	delete(got, "sum-a")

	again, ok := cache.get("cpu")
	if !ok {
		t.Fatal("expected cache entry after mutating returned map")
	}
	if _, ok := again["sum-a"]; !ok {
		t.Fatal("mutating get result removed original cache entry")
	}
	if _, ok := again["sum-b"]; ok {
		t.Fatal("mutating get result added entry to cache")
	}
}

func TestInnerCacheSnapshotReturnsCopy(t *testing.T) {
	cache := newInnerCache()
	cache.put("cpu", testConfigWithSum("sum-a"))

	snapshot := cache.snapshot()
	snapshot["mem"] = map[string]cfg.ConfigWithFormat{
		"sum-b": testConfigWithSum("sum-b"),
	}
	delete(snapshot["cpu"], "sum-a")

	again := cache.snapshot()
	if _, ok := again["cpu"]["sum-a"]; !ok {
		t.Fatal("mutating snapshot removed original cache entry")
	}
	if _, ok := again["mem"]; ok {
		t.Fatal("mutating snapshot added input to cache")
	}
}
