# Tengine Input Plugin

The tengine plugin gathers metrics from the
[Tengine Web Server](http://tengine.taobao.org/) via the
[reqstat](http://tengine.taobao.org/document/http_reqstat.html) module.

## Tengine Configuration Example

```
http {

    req_status_zone server "$host,$server_addr:$server_port" 10M;
    #req_status_zone_add_indicator server $limit;
    req_status server;
    
    server {
        location /us {
            req_status_show;
            #req_status_show_field req_total $limit;
            #allow 127.0.0.1/32;
            #deny all;
        }
        
        #set $limit 0;
        #if ($arg_limit = '1') {
        #    set $limit 1;
        #}
    }
}
```

## Metrics

- Measurement
    - tags:
        - target
        - target_port
        - server_name
        - server_schema
    - fields:
        - bytes_in (integer, total number of bytes received from client)
        - bytes_out (integer, total number of bytes sent to client)
        - conn_total (integer, total number of accepted connections)
        - req_total (integer, total number of processed requests)
        - http_2xx (integer, total number of 2xx requests)
        - http_3xx (integer, total number of 3xx requests)
        - http_4xx (integer, total number of 4xx requests)
        - http_5xx (integer, total number of 5xx requests)
        - http_other_status (integer, total number of other requests)
        - rt (integer, accumulation or rt)
        - ups_req (integer, total number of requests calling for upstream)
        - ups_rt (integer, accumulation or upstream rt)
        - ups_tries (integer, total number of times calling for upstream)
        - http_200 (integer, total number of 200 requests)
        - http_206 (integer, total number of 206 requests)
        - http_302 (integer, total number of 302 requests)
        - http_304 (integer, total number of 304 requests)
        - http_403 (integer, total number of 403 requests)
        - http_404 (integer, total number of 404 requests)
        - http_416 (integer, total number of 416 requests)
        - http_499 (integer, total number of 499 requests)
        - http_500 (integer, total number of 500 requests)
        - http_502 (integer, total number of 502 requests)
        - http_503 (integer, total number of 503 requests)
        - http_504 (integer, total number of 504 requests)
        - http_508 (integer, total number of 508 requests)
        - http_other_detail_status (integer, total number of requests of other status codes*http_ups_4xx total number of requests of upstream 4xx)
        - http_ups_5xx (integer, total number of requests of upstream 5xx)

## Example Output

```text
tengine_rt agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 37634
tengine_ups_rt agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 37394
tengine_http_499 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 0
tengine_http_504 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 0
tengine_bytes_in agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 129592
tengine_http_4xx agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=http target=127.0.0.1 target_port=80 535
tengine_http_other_status agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 0
tengine_http_200 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 14452
tengine_http_499 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 0
tengine_http_503 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 0
tengine_http_504 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 0
tengine_http_500 agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 0
tengine_http_ups_4xx agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 13
tengine_http_ups_5xx agent_hostname=zy-fat project=matrix server_name=www.baidu.com server_schema=https target=127.0.0.1 target_port=80 1
```