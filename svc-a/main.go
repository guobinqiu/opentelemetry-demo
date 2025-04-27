package main

import (
	"context"
	"io"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

func initTracer() func(context.Context) error {
	exp, _ := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint("localhost:4318"), // 对应otel-config的receiver的http的配置
		otlptracehttp.WithInsecure(),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("service-a"),
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
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

		tracer := otel.Tracer("service-a")
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
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

	mux := http.NewServeMux()
	mux.Handle("/a", traceMiddleware(http.HandlerFunc(handler)))

	log.Println("Service A running on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
