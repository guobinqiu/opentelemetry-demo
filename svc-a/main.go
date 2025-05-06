package main

import (
	"context"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	// "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	// "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	// go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc

	// "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"

	// sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	// "go.opentelemetry.io/otel/log/global"

	log "github.com/sirupsen/logrus"
)

func initTracer() func(context.Context) error {
	res, _ := sdkresource.New(context.Background(),
		sdkresource.WithAttributes(
			attribute.String("service.name", "service-a"),
			attribute.String("service.version", "v1.0.0"),
		),
	)

	// traceExporter, _ := otlptracegrpc.New(context.Background(),
	// 	otlptracegrpc.WithEndpoint("localhost:4317"), // 对应otel-config的receiver的grpc的配置
	// 	otlptracegrpc.WithInsecure(),
	// )

	traceExporter, _ := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("localhost:4318"), // 对应otel-config的receiver的http的配置
		otlptracehttp.WithInsecure(),
	)

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1))),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{}) // 重要

	return traceProvider.Shutdown
}

func initMetric() func(context.Context) error {
	// 自定义直方图用到的桶边界
	// 把各请求时长分布到不同的桶比统一算平均要准
	// 如果一个请求花了9s, 那么10 和 +inf 两个桶计数会+1
	// 如果一个请求花了0.04s, 那么0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, +inf 这些桶计数都会+1
	// 也可以通过在otel-config.yml里配置processor来自定义桶边界
	customBuckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	view := sdkmetric.NewView(
		sdkmetric.Instrument{
			Name: "http_request_duration_seconds",
			Kind: sdkmetric.InstrumentKindHistogram,
		},
		sdkmetric.Stream{
			Name:        "http_request_duration_seconds",
			Description: "The duration of the inbound HTTP request",
			Unit:        "s",
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: customBuckets,
			},
		},
	)

	res, _ := sdkresource.New(context.Background(),
		sdkresource.WithAttributes(
			attribute.String("service.name", "service-a"),
			attribute.String("service.version", "v1.0.0"),
		),
	)

	// metricExporter, _ := otlpmetricgrpc.New(context.Background(),
	// 	otlpmetricgrpc.WithEndpoint("localhost:4317"), // 对应otel-config的receiver的grpc的配置
	// 	otlpmetricgrpc.WithInsecure(),
	// )

	metricExporter, _ := otlpmetrichttp.New(context.Background(),
		otlpmetrichttp.WithEndpoint("localhost:4318"),
		otlpmetrichttp.WithInsecure(),
	)

	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(view),
	)

	otel.SetMeterProvider(metricProvider)

	return metricProvider.Shutdown
}

// func initLog() func(context.Context) error {
// 	res, _ := sdkresource.New(context.Background(),
// 		sdkresource.WithAttributes(
// 			attribute.String("service.name", "service-a"),
// 			attribute.String("service.version", "v1.0.0"),
// 		),
// 	)

// 	logExporter, _ := otlploghttp.New(context.Background(),
// 		otlploghttp.WithEndpoint("localhost:4318"),
// 		otlploghttp.WithInsecure(),
// 	)

// 	logProvider := sdklog.NewLoggerProvider(
// 		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
// 		sdklog.WithResource(res),
// 	)

// 	global.SetLoggerProvider(logProvider)

// 	return logProvider.Shutdown
// }

// server 自动 Trace 用中间件统一 Extract()
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

		tracer := otel.Tracer("service-a")
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// type HandlerFunc func(ResponseWriter, *Request)

// 统一api总请求数
// 若通过grafana里配置query为rate(http_requests_total[1m])便可求得QPS
func metricMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meter := otel.Meter("service-a")
		apiRequestCounter, _ := meter.Int64Counter("http_requests_total")

		apiRequestCounter.Add(r.Context(), 1,
			metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("path", r.URL.Path),
			),
		)
		next.ServeHTTP(w, r)
	})
}

// 统计api响应时长
func latencyMetricMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meter := otel.Meter("service-a")
		requestDuration, _ := meter.Float64Histogram("http_request_duration_seconds")

		startTime := time.Now()

		next.ServeHTTP(w, r)

		duration := time.Since(startTime).Seconds()

		requestDuration.Record(r.Context(), duration,
			metric.WithAttributes(
				attribute.String("method", r.Method),
				attribute.String("path", r.URL.Path),
			),
		)
	})
}

// 实际处理逻辑 调用service-b的接口
func handler(w http.ResponseWriter, r *http.Request) {
	log.WithField("svc", "svc-a").WithField("api", "/a").Info("this is a log")

	ctx := r.Context()

	// 请求 service-b，注入当前 trace
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://localhost:8082/b", nil)

	// 重要: 上下游对接就靠的这行代码
	// 每个http请求都这么写麻烦的很 封装一个自定义的全局的 http client 来自动 Trace 就行了
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	io.Copy(w, resp.Body)
}

func main() {
	shutdown := initTracer()
	defer shutdown(context.Background())

	shutdown2 := initMetric()
	defer shutdown2(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/a", traceMiddleware(metricMiddleware(latencyMetricMiddleware(http.HandlerFunc(handler)))))

	log.Println("Service A running on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
