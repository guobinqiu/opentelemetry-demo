package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// 初始化 Tracer
func initTracer() func(context.Context) error {
	exp, _ := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("localhost:4318"),
		otlptracehttp.WithInsecure(),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("service-b"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown
}

// server 自动 Trace 用中间件统一 Extract()
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// traceparent: 00-<trace-id>-<span-id>-<trace-flags>
		// fmt.Println("Before Extract:", r.Header)
		fmt.Println("Before Extract:", r.Header.Get("Traceparent"))

		// 从 HTTP 请求头中提取 trace 上下文信息（trace-id 和 parent span-id 等），并注入到当前的 ctx 中，形成一个新的带有上游链路信息的 context
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

		span := trace.SpanFromContext(ctx)
		fmt.Printf("After Extract: trace-id=%s, span-id=%s\n", span.SpanContext().TraceID(), span.SpanContext().SpanID())

		// 创建一个新的 span，这个新的 span 会继承上游的 trace-id 和当前的 span-id
		tracer := otel.Tracer("service-b")
		ctx, span = tracer.Start(ctx, r.Method+" "+r.URL.Path)

		fmt.Printf("After Start: trace-id=%s, span-id=%s\n", span.SpanContext().TraceID(), span.SpanContext().SpanID())

		defer span.End()

		// 将新 context 注入到请求中
		r = r.WithContext(ctx)

		// 调用下一个 handler
		next.ServeHTTP(w, r)
	})
}

// 实际处理逻辑
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello from Service B")
}

func main() {
	shutdown := initTracer()
	defer shutdown(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/b", traceMiddleware(http.HandlerFunc(handler)))

	log.Println("Service B running on :8082")
	log.Fatal(http.ListenAndServe(":8082", mux))
}
