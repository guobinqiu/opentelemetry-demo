package main

import (
	"context"
	"io"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	// "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	// "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
	return traceProvider.Shutdown
}

func initMetric() func(context.Context) error {
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
	)

	otel.SetMeterProvider(metricProvider)

	return metricProvider.Shutdown
}

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

// 实际处理逻辑 调用service-b的接口
func handler(w http.ResponseWriter, r *http.Request) {
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
	mux.Handle("/a", traceMiddleware(metricMiddleware(http.HandlerFunc(handler))))

	log.Println("Service A running on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
