module flashcat.cloud/categraf

go 1.18

require (
	github.com/BurntSushi/toml v1.1.0
	github.com/ClickHouse/clickhouse-go/v2 v2.0.15
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/Shopify/sarama v1.34.1
	github.com/chai2010/winsvc v0.0.0-20200705094454-db7ec320025c
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/docker/docker v20.10.16+incompatible
	github.com/gaochao1/sw v1.0.0
	github.com/go-kit/log v0.2.0
	github.com/go-ping/ping v0.0.0-20211130115550-779d1e919534
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-sql-driver/mysql v1.6.0
	github.com/gobwas/glob v0.2.3
	github.com/godror/godror v0.33.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4
	github.com/hashicorp/consul/api v1.13.0
	github.com/influxdata/line-protocol/v2 v2.2.1
	github.com/jmoiron/sqlx v1.3.5
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/koding/multiconfig v0.0.0-20171124222453-69c27309b2d7
	github.com/krallistic/kazoo-go v0.0.0-20170526135507-a15279744f4e
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/alibabacloudlogserviceexporter v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/jaegerexporter v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/zipkinexporter v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/spanprocessor v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver v0.54.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver v0.54.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/prometheus/client_golang v1.12.2
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.35.0
	github.com/prometheus/prometheus v2.5.0+incompatible
	github.com/shirou/gopsutil/v3 v3.22.5
	github.com/stretchr/testify v1.7.4
	github.com/toolkits/pkg v1.3.0
	github.com/ulricqin/gosnmp v0.0.1
	github.com/xdg/scram v1.0.5
	go.opentelemetry.io/collector v0.54.0
	go.opentelemetry.io/otel/metric v0.30.0
	go.opentelemetry.io/otel/trace v1.7.0
	go.uber.org/multierr v1.8.0
	go.uber.org/zap v1.21.0
	golang.org/x/net v0.0.0-20220607020251-c690dde0001d
	golang.org/x/sys v0.0.0-20220610221304-9f5ed59c137d
	golang.org/x/text v0.3.7
	gopkg.in/yaml.v3 v3.0.1
	sigs.k8s.io/yaml v1.2.0
)

require (
	cloud.google.com/go/compute v1.6.1 // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.1 // indirect
	github.com/GehirnInc/crypt v0.0.0-20200316065508-bb7000b8a962 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/VividCortex/gohistogram v1.0.0 // indirect
	github.com/aliyun/aliyun-log-go-sdk v0.1.36 // indirect
	github.com/alouca/gologger v0.0.0-20120904114645-7d4b7291de9c // indirect
	github.com/antonmedv/expr v1.9.0 // indirect
	github.com/apache/thrift v0.16.0 // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/aws/aws-sdk-go v1.44.38 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/census-instrumentation/opencensus-proto v0.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/eapache/go-resiliency v1.2.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.2 // indirect
	github.com/freedomkk-qfeng/go-fastping v0.0.0-20160109021039-d7bb493dee3e // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-kit/kit v0.11.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godror/knownpb v0.1.0 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.2.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/serf v0.9.7 // indirect
	github.com/huandu/xstrings v1.3.1 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jaegertracing/jaeger v1.35.2 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.0.0 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.2 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.15.6 // indirect
	github.com/knadh/koanf v1.4.2 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mostynb/go-grpc-compression v1.1.16 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.54.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.54.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/opencensus v0.54.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.54.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.0 // indirect
	github.com/paulmach/orb v0.7.1 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/pelletier/go-toml/v2 v2.0.0-beta.8 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rs/cors v1.8.2 // indirect
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/cobra v1.4.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.11.0 // indirect
	github.com/stretchr/objx v0.4.0 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/tg123/go-htpasswd v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.1 // indirect
	github.com/xdg-go/stringprep v1.0.3 // indirect
	github.com/xdg/stringprep v1.0.3 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.opentelemetry.io/collector/pdata v0.54.0 // indirect
	go.opentelemetry.io/collector/semconv v0.54.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.32.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.32.0 // indirect
	go.opentelemetry.io/otel v1.7.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.30.0 // indirect
	go.opentelemetry.io/otel/sdk v1.7.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.30.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/automaxprocs v1.5.1 // indirect
	golang.org/x/crypto v0.0.0-20220507011949-2cf3adece122 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	google.golang.org/genproto v0.0.0-20220608133413-ed9918b62aac // indirect
	google.golang.org/grpc v1.47.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/ini.v1 v1.66.4 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools/v3 v3.2.0 // indirect
)

replace (
	github.com/prometheus/client_golang => github.com/flashcatcloud/client_golang v1.12.2-0.20220704074148-3b31f0c90903
	go.opentelemetry.io/collector => github.com/flashcatcloud/opentelemetry-collector v0.54.1-0.20220628041301-3b8dabd1bcd0
)
