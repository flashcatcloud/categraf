# prometheus
Categraf simply wrapped prometheus agent mode fuction, which means you can scrape metrics like prometheus do and get full support with prometheus plugins, such as service discovery and relabel.

For more details, see the official docs:
- https://github.com/prometheus/prometheus/tree/main/documentation/examples

## Configuration

An [example](../k8s/in_cluster_scrape.yaml) to scrape kube-apiserver and core-dns metrics .
more examples click [here](https://github.com/prometheus/prometheus/tree/main/documentation/examples)


## How to create token 

1. crate token ```kubectl apply -f auth.yaml```,  replace CATEGRAF_NAMESPACE with your own in auth.yaml 
```
### auth.yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations: {}
  labels:
    app: n9e
    component: categraf
  name: categraf-role
rules:
  - apiGroups: [""]
    resources:
      - nodes
      - nodes/metrics
      - services
      - endpoints
      - pods
    verbs: ["get", "list", "watch"]
  - apiGroups:
      - extensions
      - networking.k8s.io
    resources:
      - ingresses
    verbs: ["get", "list", "watch"]
  - nonResourceURLs: ["/metrics", "/metrics/cadvisor"]
    verbs: ["get"]
---
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations: {}
  labels:
    app: n9e
    component: categraf
  name: categraf-serviceaccount
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  annotations: {}
  labels:
    app: n9e
    component: categraf
  name: categraf-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: categraf-role
subjects:
- kind: ServiceAccount
  name: categraf-serviceaccount
  namespace: ${CATEGRAF_NAMESPACE}
---
```
2. get token
Recommended Strongly: Scraping in cluster, token will be auto mount into pod with path ```/var/run/secrets/kubernetes.io/serviceaccount/token```, you do not need to care about it. Replace all Vars with your own in file `k8s/in_cluster_scrape.yaml`.
 
Scraping out of cluster, you can get token with this way and save it to file, then fill `bearer_token_file` in file `k8s/scrape_with_token.yaml` 
``` 
   secrets=$(kubectl get serviceaccount categraf-serviceaccount -o jsonpath={.secrets[].name})
   kubectl get secrets ${secrets} -o jsonpath={.data.token} | base64 -d
``` 
`k8s/scrape_with_cafile.yaml` and `k8s/scrape_with_kubecofnig.yaml` is recommended only if you are proficient in 
X509 client certs.

