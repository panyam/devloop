package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func initTracing() {
	ctx := context.Background()
	
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint("http://localhost:4318"),
	)
	if err != nil {
		log.Printf("Failed to create OTLP exporter: %v", err)
		return
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(trace.WithAttributes(
			semconv.ServiceNameKey.String("backend-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
		)),
	)
	
	otel.SetTracerProvider(tp)
	log.Println("OpenTelemetry tracing initialized")
}

func main() {
	initTracing()
	
	tracer := otel.Tracer("backend-service")
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "handle-request")
		defer span.End()
		
		// Simulate some work
		time.Sleep(50 * time.Millisecond)
		
		fmt.Fprintf(w, "Hello from backend service! Request handled at %s\n", 
			time.Now().Format(time.RFC3339))
		
		log.Printf("Request handled: %s %s", r.Method, r.URL.Path)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status": "healthy", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
	})

	log.Println("Starting backend service on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}