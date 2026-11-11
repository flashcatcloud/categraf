# monitoring kubernetes control plane with plugin prometheus


## if your control plane is in pod, for example, you use kubeadm build k8s cluster. Then kube-controller-manager, kube-scheduler and etcd need some extra work to be discovery.

### create service for kube-controller-manager
1. `kubectl apply -f controller-service.yaml` 
2. edit `/etc/kubernetes/manifests/kube-controller-manager.yaml` , modify or add one line `- --bind-address=0.0.0.0`  
3. wait kube-controller-manager to restart
 
### create service for kube-scheduler
3. `kubectl apply -f scheduler-service.yaml`
4. edit `/etc/kubernetes/manifests/kube-scheduler.yaml` , modify or add one line `- --bind-address=0.0.0.0`
5. wait kube-scheduler to restart

### create service for etcd
6. `kubectl apply -f etcd-service-http.yaml`
7. edit `/etc/kubernetes/manifests/etcd.yaml` ,  modify `- --listen-metrics-urls=http://127.0.0.1:2381` to `- --listen-metrics-urls=http://0.0.0.0:2381`
8. wait etcd to restart

### create all other objects with deployment
9. edit deployment.yaml and modify it with your own configure.
 
   i. replace ${CATEGRAF_NAMESPACE} which located in ClusterRoleBinding part
 
   ii. replace ${NSERVER_SERVICE_WITH_PORT} which located in ConfigMap part config.toml and in_cluster_scrape.yaml

   if you choose `etcd-service.yaml` with https mode, then `kubectl apply -f etcd-service.yaml`.
 
   iii. replace `{data of your etcd ca file}` `{data of your etcd client cert file}` `{data of your etcd client key file}` in ConfigMap etcd-pki.

 
10. `kubectl apply -f  deployment-etcd-http.yaml -n monitoring`

Make sure that `deployment.yaml` always appears with `etcd-service.yaml` and `deployment-etcd-http` appears with `etcd-service-http.yaml`. They cannot be apply at the same time.

## Use IPv4 for Agent connections

For IPv4-only deployments, set `PREFER_IPV4_STACK` on the Categraf container to
avoid AAAA DNS queries on the supported connection and address-discovery paths:

```yaml
spec:
  template:
    spec:
      containers:
        - name: categraf
          # Keep the existing image, arguments, mounts, and other environment variables.
          env:
            - name: PREFER_IPV4_STACK
              value: "true"
```

This is a Pod template fragment for a Deployment, DaemonSet, or StatefulSet.
For a standalone Pod, set the variable under `spec.containers`. In sidecar mode,
set it on the Categraf container. Roll out updated Pods to apply the change.

Only the exact string `true` enables this behavior. The supported paths are:

- All writers and the enabled heartbeat client.
- Outbound host-IP detection at startup and every minute when `HOSTIP` is unset.
- Remote HTTP configuration, Ibex RPC, and upgrade downloads.
- Logs sent through HTTP, TCP, SOCKS, and direct Kafka connections; remote Docker
  log connections, Kubelet address discovery, and Kubelet HTTP requests.
- Public `pkg/httpx` and `pkg/proxy` clients, including `http_response`, NSQ,
  JSON exporter, and the related `x509_cert` TCP paths.

IPv6-only targets on these paths fail without IPv6 fallback. Explicit address
families and dedicated IPv6/DNS monitoring retain their own settings. Independent
plugin clients, database/cloud SDKs, and the embedded Prometheus subsystem are
not all covered; this switch does not guarantee that the entire Pod stops AAAA
queries. See the [scope and implementation plan](../docs/issue-1513-ipv4-scope-and-fix-plan.md).

With a proxy, the policy applies to the supported local connection to the proxy.
DNS performed by the proxy and custom Kafka proxy dialers retain their own policy.
When Kafka `Net.ResolveCanonicalBootstrapServers` is enabled, direct bootstrap
discovery uses A records and retains PTR-based canonical hostnames for TLS/GSSAPI.
Combining that setting with both a custom proxy dialer and the IPv4 switch is
rejected explicitly, because the proxy may own remote name resolution.

Outbound IP selection can change heartbeat `host_ip`, `$ip`-based hostnames and
labels. Check host identity and time-series continuity during rollout. Setting
the correct `HOSTIP` for each Categraf instance bypasses outbound IP detection.

Existing connection pooling, HTTP protocol selection, and TLS verification are
preserved. Remove the variable or set it to `"false"` and roll out updated Pods
to restore the default connection policy. Review any host identity changes
separately when rolling back.

This removes AAAA queries from the affected IPv4 dialing paths, not A queries or
queries from other containers. For large clusters, also review Pod DNS search
domains/`ndots` and consider NodeLocal DNSCache to reduce shared DNS load.

# dashboards show
___
![apiserver-dashboards](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/k8s/images/apiserver-dash.jpg)
___
![controller-dashboards](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/k8s/images/cm-dash.jpg)
___
![scheduler-dashboards](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/k8s/images/scheduler-dash.jpg)
___
![etcd-dashboards](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/k8s/images/etcd-dash.jpg)
___
![coredns-dashboards](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/k8s/images/coredns-dash.jpg)
