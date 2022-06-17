package writer

import (
	"errors"
	"log"
	"time"

	"flashcat.cloud/categraf/config"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var ClickHouseWriterChan chan *driver.Conn

func InitClickHouseConnect(opts []config.WriterOption) error {

	ClickHouseWriterChan = make(chan *driver.Conn, 100)
	for i := 0; i < len(opts); i++ {
		if opts[i].StorageType == "OLAP" {
			if len(opts[i].ClickHouseEndpoints) == 0 {

				return errors.New("At least one clickhouse endpoint")
			}
			for _, clickhouseEndpoint := range opts[i].ClickHouseEndpoints {
				conn, err := clickhouse.Open(&clickhouse.Options{
					Addr: []string{clickhouseEndpoint},
					Auth: clickhouse.Auth{
						Database: opts[i].ClickHouseDB,
						Username: opts[i].BasicAuthUser,
						Password: opts[i].BasicAuthPass,
					},
					//Debug:           true,
					DialTimeout:     time.Second,
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: time.Hour,
				})
				if err != nil {
					log.Println("W! Connect ClickHouse fail:", err.Error())
					continue
				}
				ClickHouseWriterChan <- &conn
				if len(ClickHouseWriterChan) == 0 {
					return errors.New("All endpoints can not connect")
				}
			}
			// 只支持一个clickhouse集群配置
			break
		}
	}

	return nil
}
