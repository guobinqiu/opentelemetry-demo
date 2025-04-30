# OpenTelemetry(otel) Demo

| traces                        | metrics                     | logs                  |
| ----------------------------- | --------------------------- | --------------------- |
| otel + jaeger + elasticsearch | otel + prometheus + grafana | otel + loki + grafana |

> 注意: 这里有个巨坑 单机使用badger无法让jaeger-collector和jaeger-query共享存储 后果就是配置没有问题但是 jaeger-query 无论如何看不到 trace 数据 这里使用elasticsearch作为jaeger的后端存储

![image](https://github.com/user-attachments/assets/565d5d36-dddb-4895-ad84-a8e47e62f310)

各服务默认端口列表

| 端口  | 说明                                    |
| ----- | --------------------------------------- |
| 4317  | OTLP gRPC                               |
| 4318  | OTLP HTTP                               |
| 14250 | Jaeger gRPC                             |
| 14268 | Jaeger HTTP                             |
| 16686 | Jaeger UI                               |
| 9090  | Prometheus UI                           |
| 9200  | Elasticsearch (ES) - HTTP API           |
| 9300  | Elasticsearch (ES) - 集群节点间通信端口 |
| 3000  | Grafana UI                              |

## docker 安装

### all-in-one 模式

1.配置 `config/docker/allinone/otelcol-config.yml`

```
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: otel-collector:4317 # 通过grpc从app接收遥测数据
      http:
        endpoint: otel-collector:4318 # 通过http从app接收遥测数据
exporters:
  otlp:
    endpoint: "jaeger:4317"
    tls:
      insecure: true
  debug:
  prometheus:
    endpoint: "0.0.0.0:9464" # 让Prometheus来访问这个端口
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp, debug]
    metrics:
      receivers: [otlp]
      exporters: [prometheus]
```

2.配置 `config/docker/allinone/prometheus.yml`

```
scrape_configs:
  - job_name: 'otel-collector'
    static_configs:
      - targets: ['otel-collector:9464']
```

3.配置 `config/docker/allinone/docker-compose.yaml`

```
services:
  otel-collector:
    image: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib:0.120.0
    container_name: otel-collector
    command: "--config=/etc/otelcol-config.yml"
    volumes:
      - ./otelcol-config.yml:/etc/otelcol-config.yml
    ports:
      - "4317:4317" # OTLP gRPC
      - "4318:4318" # OTLP HTTP
    depends_on:
      - jaeger

  jaeger:
    image: jaegertracing/all-in-one:1.66.0
    container_name: jaeger
    ports:
      - "16686:16686" # Jaeger UI

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
```

### standalone 模式

1.配置 `config/docker/standalone/otel-config.yml`

```
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: otel-collector:4317 # 通过grpc从app接收遥测数据
      http:
        endpoint: otel-collector:4318 # 通过http从app接收遥测数据
exporters:
  otlp:
    endpoint: jaeger-collector:4317 # 导出到jaeger-collector的gRPC端口
    tls:
      insecure: true
  debug:
  prometheus:
    endpoint: "0.0.0.0:9464" # 让Prometheus来访问这个端口
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp, debug]
    metrics:
      receivers: [otlp]
      exporters: [prometheus]
```

2.配置 `config/docker/standalone/prometheus.yml`

```
scrape_configs:
  - job_name: 'otel-collector'
    static_configs:
      - targets: ['otel-collector:9464']
```

3.配置 `config/docker/standalone/docker-compose.yaml`

```
services:
  otel-collector:
    image: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib:0.120.0
    container_name: otel-collector
    command: "--config=/etc/otelcol-config.yml"
    volumes:
      - ./otelcol-config.yml:/etc/otelcol-config.yml
    ports:
      - "4317:4317" # OTLP gRPC
      - "4318:4318" # OTLP HTTP
    depends_on:
      - jaeger-collector

  jaeger-collector:
    image: jaegertracing/jaeger-collector:1.66.0
    container_name: jaeger-collector
    environment:
      - SPAN_STORAGE_TYPE=elasticsearch
      - ES_SERVER_URLS=http://elasticsearch:9200
    depends_on:
      - elasticsearch
    restart: always

  jaeger-query:
    image: jaegertracing/jaeger-query:1.66.0
    container_name: jaeger-query
    environment:
      - SPAN_STORAGE_TYPE=elasticsearch
      - ES_SERVER_URLS=http://elasticsearch:9200
    ports:
      - "16686:16686"
    depends_on:
      - elasticsearch
    restart: always

  elasticsearch:
    image: elasticsearch:7.17.28
    container_name: elasticsearch
    environment:
      - discovery.type=single-node
    ports:
      - "9200:9200"
      - "9300:9300"

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml  
```

## 二进制文件 安装

### 下载

[otel-collector](https://github.com/open-telemetry/opentelemetry-collector-releases/releases)

[jaeger](https://github.com/jaegertracing/jaeger/releases)

[elasticsearch](https://github.com/elastic/elasticsearch/releases)

[prometheus](https://github.com/prometheus/prometheus/releases)

[loki](https://github.com/grafana/loki/releases)

[grafana](https://github.com/grafana/grafana/releases)

可根据需要自行下载指定版本到bin目录下

1.配置 `config/otelcol-config.yml`

```
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: localhost:4317 # 通过grpc从app接收遥测数据
      http:
        endpoint: localhost:4318 # 通过http从app接收遥测数据
exporters:
  otlp:
    endpoint: localhost:43170 # 导出到jaeger-collector的gRPC端口
    tls:
      insecure: true
  debug:
  prometheus:
    endpoint: "localhost:9464" #让 Prometheus 来访问这个端口
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp, debug]
    metrics:
      receivers: [otlp]
      exporters: [prometheus]
```

2.配置 `config/prometheus.yml`

```
scrape_configs:
  - job_name: 'otel-collector'
    static_configs:
      - targets: ['localhost:9464']
```

3.运行 `otel-collector`

```
./bin/otelcol-contrib --config config/otelcol-config.yml
```

4.运行 `jaeger-collector`

由于 `otel-collector` 默认已经启动在了 `4317` 和 `4318` 端口, 如果在同一台机器上启动 `jaeger-collector` 需要修改otlp对应的端口为 `43170` 和 `43180` 来避免端口冲突

```
export SPAN_STORAGE_TYPE=elasticsearch
export ES_SERVER_URLS=http://localhost:9200
./bin/jaeger-collector \
  --collector.otlp.enabled=true \
  --collector.otlp.grpc.host-port=:43170 \
  --collector.otlp.http.host-port=:43180
```

5.运行 `jaeger-query`

```
export SPAN_STORAGE_TYPE=elasticsearch
export ES_SERVER_URLS=http://localhost:9200
./bin/jaeger-query
```

## jaeger UI

http://locahost:16686

`svc-a` 的api `/a` 调用 `svc-b` 的 api `/b` 查看跟踪数据

## prometheus

http://localhost:9090

统计`svc-a` 的api `/a` 被调用的次数

## loki

待更新
