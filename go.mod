module flashcat.cloud/categraf

godebug x509negativeserial=1

go 1.24.3

require (
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137
	github.com/chai2010/winsvc v0.0.0-20200705094454-db7ec320025c
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/docker/docker v24.0.9+incompatible
	github.com/emirpasic/gods v1.18.1
	github.com/gaochao1/sw v1.0.0
	github.com/gin-gonic/gin v1.9.1
	github.com/go-kit/log v0.2.1
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-sql-driver/mysql v1.6.0
	github.com/gobwas/glob v0.2.3
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.3
	github.com/golang/snappy v0.0.4
	github.com/gosnmp/gosnmp v1.37.0
	github.com/hashicorp/consul/api v1.15.3
	github.com/influxdata/line-protocol/v2 v2.2.1
	github.com/jackc/pgx/v4 v4.18.2
	github.com/jcmturner/gokrb5/v8 v8.4.4
	github.com/json-iterator/go v1.1.12
	github.com/koding/multiconfig v0.0.0-20171124222453-69c27309b2d7
	github.com/krallistic/kazoo-go v0.0.0-20170526135507-a15279744f4e
	github.com/mattn/go-isatty v0.0.20
	github.com/matttproud/golang_protobuf_extensions v1.0.4
	github.com/miekg/dns v1.1.50
	github.com/moby/ipvs v1.0.2
	github.com/oklog/run v1.1.0
	github.com/orcaman/concurrent-map v1.0.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.18.0
	github.com/prometheus/client_model v0.6.0
	github.com/prometheus/common v0.47.0
	github.com/prometheus/prometheus v0.40.0
	github.com/shirou/gopsutil/v3 v3.22.5
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
	github.com/toolkits/pkg v1.3.7
	github.com/ulricqin/gosnmp v0.0.1
	github.com/xdg/scram v1.0.5
	go.mongodb.org/mongo-driver v1.10.2
	go.opentelemetry.io/otel/metric v1.18.0 // indirect
	go.opentelemetry.io/otel/trace v1.18.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/net v0.38.0
	golang.org/x/sys v0.31.0
	golang.org/x/text v0.23.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/Azure/go-ntlmssp v0.0.0-20221128193559-754e69321358 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/alecthomas/participle v0.4.1 // indirect
	github.com/alibabacloud-go/alibabacloud-gateway-spi v0.0.4 // indirect
	github.com/alibabacloud-go/debug v0.0.0-20190504072949-9472017b5c68 // indirect
	github.com/alibabacloud-go/endpoint-util v1.1.0 // indirect
	github.com/alibabacloud-go/openapi-util v0.0.11 // indirect
	github.com/alibabacloud-go/tea-utils v1.3.1 // indirect
	github.com/alibabacloud-go/tea-utils/v2 v2.0.0 // indirect
	github.com/alibabacloud-go/tea-xml v1.1.2 // indirect
	github.com/aliyun/credentials-go v1.2.6 // indirect
	github.com/awnumar/memcall v0.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.28 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.1 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/bytedance/sonic v1.9.1 // indirect
	github.com/chenzhuoyu/base64x v0.0.0-20221115062448-fe3a3abad311 // indirect
	github.com/clbanning/mxj/v2 v2.5.5 // indirect
	github.com/dennwc/ioctl v1.0.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/frankban/quicktest v1.14.3 // indirect
	github.com/gabriel-vasile/mimetype v1.4.2 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.5 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/likexian/gokit v0.25.13 // indirect
	github.com/mdlayher/genetlink v1.3.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/montanaflynn/stats v0.0.0-20171201202039-1bf9dbcd8cbe // indirect
	github.com/nats-io/jwt/v2 v2.7.3 // indirect
	github.com/nats-io/nkeys v0.4.10 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ovh/go-ovh v1.1.0 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/transport/v2 v2.2.10 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/siebenmann/go-kstat v0.0.0-20210513183136-173c9b0a9973 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tjfoc/gmsm v1.3.2 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	golang.org/x/arch v0.3.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)

require (
	cloud.google.com/go/monitoring v1.16.3
	github.com/AlekSi/pointer v1.2.0
	github.com/IBM/sarama v1.42.1
	github.com/Knetic/govaluate v3.0.1-0.20171022003610-9aa49832a739+incompatible
	github.com/NVIDIA/go-dcgm v0.0.0-20240118201113-3385e277e49f
	github.com/NVIDIA/go-nvml v0.12.0-2
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/alibabacloud-go/cms-20190101/v8 v8.0.0
	github.com/alibabacloud-go/cms-export-20211101/v2 v2.0.0
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.0.0
	github.com/alibabacloud-go/tea v1.1.19
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/awnumar/memguard v0.22.4
	github.com/aws/aws-sdk-go-v2 v1.17.4
	github.com/aws/aws-sdk-go-v2/config v1.18.12
	github.com/aws/aws-sdk-go-v2/credentials v1.13.12
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.5.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.18.3
	github.com/beevik/ntp v1.3.1
	github.com/bits-and-blooms/bitset v1.13.0
	github.com/blang/semver/v4 v4.0.0
	github.com/bmatcuk/doublestar/v3 v3.0.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/dennwc/btrfs v0.0.0-20230312211831-a1f570bd01a1
	github.com/ema/qdisc v1.0.0
	github.com/go-ldap/ldap/v3 v3.4.6
	github.com/godbus/dbus/v5 v5.0.4
	github.com/google/gnxi v0.0.0-20240912171544-ef18504847b0
	github.com/hashicorp/go-envparse v0.1.0
	github.com/hodgesds/perf-utils v0.7.0
	github.com/illumos/go-kstat v0.0.0-20210513183136-173c9b0a9973
	github.com/jsimonetti/rtnetlink v1.4.1
	github.com/kardianos/service v1.2.2
	github.com/karrick/godirwalk v1.10.3
	github.com/likexian/whois v1.15.0
	github.com/likexian/whois-parser v1.24.8
	github.com/lufia/iostat v1.2.1
	github.com/mattn/go-xmlrpc v0.0.3
	github.com/mdlayher/ethtool v0.1.0
	github.com/mdlayher/netlink v1.7.2
	github.com/mdlayher/wifi v0.1.0
	github.com/nats-io/nats-server/v2 v2.10.27
	github.com/openconfig/gnmi v0.11.0
	github.com/opencontainers/selinux v1.11.0
	github.com/percona/percona-toolkit v0.0.0-20211210121818-b2860eee3152
	github.com/pion/dtls/v2 v2.2.12
	github.com/prometheus-community/go-runit v0.1.0
	github.com/prometheus-community/pro-bing v0.1.0
	github.com/safchain/ethtool v0.3.0
	github.com/sijms/go-ora/v2 v2.8.6
	github.com/sleepinggenius2/gosmi v0.4.4
	github.com/tidwall/gjson v1.14.4
	github.com/vmware/govmomi v0.29.0
	golang.org/x/exp v0.0.0-20230713183714-613f0c0eb8a1
	google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17
	howett.net/plist v1.0.1
	k8s.io/kubelet v0.29.2
)

replace gopkg.in/yaml.v2 => github.com/rfratto/go-yaml v0.0.0-20211119180816-77389c3526dc

require (
	github.com/Azure/azure-sdk-for-go v65.0.0+incompatible // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.28 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/BurntSushi/toml v1.1.0 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/alouca/gologger v0.0.0-20120904114645-7d4b7291de9c // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/aws/aws-sdk-go v1.44.128 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/cncf/xds/go v0.0.0-20231109132714-523115ebc101 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/denisenkom/go-mssqldb v0.12.2
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/digitalocean/godo v1.88.0 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.5.0 // indirect
	github.com/eapache/go-resiliency v1.4.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/envoyproxy/go-control-plane v0.11.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.0.2 // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/freedomkk-qfeng/go-fastping v0.0.0-20160109021039-d7bb493dee3e
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/analysis v0.21.2 // indirect
	github.com/go-openapi/errors v0.20.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.1 // indirect
	github.com/go-openapi/spec v0.20.5 // indirect
	github.com/go-openapi/strfmt v0.21.3 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-openapi/validate v0.21.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.14.0 // indirect
	github.com/go-resty/resty/v2 v2.1.1-0.20191201195748-d7b97669fe48 // indirect
	github.com/go-zookeeper/zk v1.0.3 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang-sql/civil v0.0.0-20190719163853-cb61b32ac6fe // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/gophercloud/gophercloud v1.0.0 // indirect
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grafana/regexp v0.0.0-20221005093135-b4c2bcb0a4b6
	github.com/hashicorp/cronexpr v1.1.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20221102143410-8a95f1239005 // indirect
	github.com/hashicorp/serf v0.9.7 // indirect
	github.com/hetznercloud/hcloud-go v1.35.3 // indirect
	github.com/imdario/mergo v0.3.12
	github.com/ionos-cloud/sdk-go/v6 v6.1.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/linode/linodego v1.9.3 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mailru/easyjson v0.7.7
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.0.8 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/alertmanager v0.24.0 // indirect
	github.com/prometheus/common/assets v0.2.0 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/exporter-toolkit v0.11.0
	github.com/prometheus/procfs v0.12.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.9 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/tomasen/fcgi_client v0.0.0-20180423082037-2bb3d819fd19
	github.com/ugorji/go/codec v1.2.11
	github.com/vishvananda/netlink v1.1.0 // indirect
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xdg/stringprep v1.0.3 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0 // indirect
	go.opentelemetry.io/otel v1.18.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	golang.org/x/crypto v0.36.0
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/oauth2 v0.27.0 // indirect
	golang.org/x/sync v0.12.0
	golang.org/x/term v0.30.0 // indirect
	golang.org/x/time v0.10.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	google.golang.org/api v0.149.0
	google.golang.org/genproto v0.0.0-20231106174013-bbf56f31fb17
	google.golang.org/grpc v1.61.1
	google.golang.org/protobuf v1.33.0
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/api v0.29.2
	k8s.io/apimachinery v0.29.2
	k8s.io/client-go v0.29.2
	k8s.io/klog/v2 v2.110.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240220201932-37d671a357a5 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/googleapis/google-cloud-go/storage => cloud.google.com/go/storage v1.30.1
	github.com/prometheus/client_golang => github.com/flashcatcloud/client_golang v1.12.2-0.20220704074148-3b31f0c90903
	go.opentelemetry.io/collector => github.com/open-telemetry/opentelemetry-collector v0.54.0
)
