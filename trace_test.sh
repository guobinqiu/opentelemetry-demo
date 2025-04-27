#!/bin/bash
# 独立测试 jaeger collector 的可用性

git clone https://github.com/open-telemetry/opentelemetry-proto.git

go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

curl -X POST http://localhost:43180/v1/traces \
  -H "Content-Type: application/json" \
  --data-binary @trace_data_http.json

grpcurl -plaintext \
  -proto ./opentelemetry-proto/opentelemetry/proto/collector/trace/v1/trace_service.proto \
  -import-path ./opentelemetry-proto \
  -d @ \
  localhost:43170 \
  opentelemetry.proto.collector.trace.v1.TraceService/Export < trace_data_grpc.json