# prometheus
Categraf simply wrapped prometheus agent mode fuction, which means you can scrape metrics like prometheus do and get full support with prometheus plugins, such as service discovery and relabel.

For more details, see the official docs:
- https://github.com/prometheus/prometheus/tree/main/documentation/examples

## Configuration

An [example](../conf/in_cluster_scrape.yaml) to scrape kube-apiserver and core-dns metrics .
more examples click [here](https://github.com/prometheus/prometheus/tree/main/documentation/examples)