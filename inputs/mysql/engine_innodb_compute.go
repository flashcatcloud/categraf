package mysql

import (
	"database/sql"

	"flashcat.cloud/categraf/pkg/tagx"
	"flashcat.cloud/categraf/types"
)

func (ins *Instance) gatherEngineInnodbStatusCompute(slist *types.SampleList, db *sql.DB, globalTags map[string]string, cache map[string]float64) {
	tags := tagx.Copy(globalTags)

	pageUsed := cache["innodb_buffer_pool_pages_total"] - cache["innodb_buffer_pool_pages_free"]
	byteUsed := pageUsed * cache["innodb_page_size"]
	byteData := cache["innodb_buffer_pool_pages_data"] * cache["innodb_page_size"]
	byteDirty := cache["innodb_buffer_pool_pages_dirty"] * cache["innodb_page_size"]
	byteFree := cache["innodb_buffer_pool_pages_free"] * cache["innodb_page_size"]
	byteTotal := cache["innodb_buffer_pool_pages_total"] * cache["innodb_page_size"]
	pageUtil := float64(0)
	if cache["innodb_buffer_pool_pages_total"] != 0 {
		pageUtil = pageUsed / cache["innodb_buffer_pool_pages_total"] * 100
	}

	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_bytes_used", byteUsed, tags))
	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_bytes_data", byteData, tags))
	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_bytes_free", byteFree, tags))
	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_bytes_total", byteTotal, tags))
	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_bytes_dirty", byteDirty, tags))
	slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_pages_utilization", pageUtil, tags))

	if ins.ExtraInnodbMetrics {
		slist.PushFront(types.NewSample(inputName, "global_status_buffer_pool_pages_used", pageUsed, tags))
	}
}
