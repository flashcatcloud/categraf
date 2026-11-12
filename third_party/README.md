# Per-client network policy dependency patches

These local module replacements supply hooks that the pinned SDK versions do not expose. Each directory retains runtime packages, the auxiliary packages required by the transitive dependency test graph, upstream license files and the original `go.mod`/`go.sum`. The small Azure azcore module retains all non-test sources. Kazoo predates Go modules and has a minimal module declaration. This is a source snapshot, not a complete upstream development checkout.

`network-patches.json` fixes the upstream module versions and Go checksums, lists every modified source file and links its unified patch under `patches/`. Unchanged files are retained for reproducible module-mode builds. The root `go.mod` selects these directories with `replace`; the repository's generated `vendor/` remains ignored, as before. Running `go mod vendor` reproduces the same patched implementations. No fork needs publishing and no generated vendor-only edit is required. `auxiliary_packages` explicitly records the gRPC and AWS test-helper closure so `go mod tidy` can load dependency tests; CI checks `go mod tidy -diff` as well as runtime builds. Snapshot verification downloads upstream versions outside the root module and leaves `go.mod`/`go.sum` unchanged.

| Dependency | Added per-client capability |
| --- | --- |
| govmomi | Configure the SDK-owned Transport in place; inherit it in service/tunnel clients and close all owned pools on SDK cleanup |
| gRPC | Native direct/proxy network selection, private proxy DNS builder and host lookup hook while retaining SRV/TXT, service config and native CONNECT; cancel blocked CONNECT handshakes |
| Azure azcore | Fresh native HTTP client with a per-client network policy before DNS, retaining SDK TLS/HTTP2/timeout defaults; Azure discovery and remote-write owners close private credential pools |
| AWS SDK config | Per-load container endpoint hostname validation hook; retain native all-address loopback/ECS/EKS validation |
| AWS SDK core | Final transport configurer retained by BuildableClient clones, defaults mode, CA configuration and IMDS timeout specialization |
| go-mssqldb | Connector lookup hook before native address racing, and network selection for TCP and SQL Browser UDP |
| Tea / Darabonba OpenAPI | Per-client HTTP pools and final transport policy, including SOCKS first-hop network selection; ignore private policy fields during serialization |
| Kazoo | Pass HostProvider and Dialer options to its ZooKeeper client |
| Sarama / gokrb5 | Per-client KDC network selection before UDP/TCP resolution; retain discovery, timeouts and auth settings; correct RFC 4757 key-usage encoding to fixed-width little endian |
| Google API / auth / metadata / oauth2 | Credential HTTP policy after native TLS configuration, private metadata probing for both legacy and new ADC, subscription timeout preservation and external identity/mTLS client hooks |
| go-ora | Connector network selection carried into session connections and redirects without replacing native timeout/TLS logic |
| Prometheus | Carry instance policy through notifier, service discovery and remote storage; apply it before round-robin DNS and to independent signing/identity clients |
| Prometheus common / sigv4 | Independent OAuth dial options for token refreshes; inject AWS config options before constructing credential providers |

In Prometheus AWS discovery, automatic region lookup moves from YAML decoding to discoverer construction. This is necessary to supply the instance's policy before IMDS network activity; an unresolved region is now reported when the discoverer is created. All service calls still use the original HTTP/TLS/auth configuration. Each AWS discoverer also receives the credential options before its own config load, including explicit-region configurations. Container credentials require both the preflight hostname hook and the independent endpoint-provider HTTP option.

Google's opt-in experimental S2A and DirectPath transports have separate private resolver/transport contracts. Combining them with the new HTTP policy returns an explicit construction error; the policy does not silently disable those modes or claim they are controlled. Ordinary TLS and certificate-based mTLS are retained. Arbitrary user-provided resolver, dialer, token provider and external credential executable traffic retain their own contract.

Verify fixed sources and patches using Go 1.25 and Python 3:

```sh
go mod tidy -diff
python3 scripts/verify-network-patches.py
go mod tidy -diff
go mod vendor
go test -mod=readonly ./pkg/grpcx ./pkg/aws ./pkg/googlex ./inputs/vsphere ./inputs/aliyun/internal/manager ./pkg/kafkax ./prometheus github.com/prometheus/prometheus/discovery/azure
```

When upgrading a dependency, rebase only its listed changes, refresh the snapshot/version/checksums and review patch, then run the DNS, protocol, proxy, credential-refresh, pool-cleanup and cross-platform checks. Do not edit `vendor/` as the source of truth or add process-global SDK settings.
