# OpenTelemetry Integration Example

This example demonstrates how to integrate OpenTelemetry with devloop for unified observability across multiple services.

## Setup

1. **Install OpenTelemetry Collector**:
   ```bash
   # macOS
   brew install opentelemetry-collector
   
   # Or download binary
   curl -L -o otelcol \
     https://github.com/open-telemetry/opentelemetry-collector-releases/releases/latest/download/otelcol_darwin_amd64
   chmod +x otelcol
   sudo mv otelcol /usr/local/bin/
   ```

2. **Run the example**:
   ```bash
   cd examples/otel-integration
   devloop
   ```

## What This Example Shows

### Services
- **otel-agent**: OpenTelemetry Collector receiving telemetry from all services
- **backend**: Go service with OTel tracing and metrics
- **frontend**: Node.js service with OTel browser instrumentation
- **worker**: Python service with OTel logging and tracing

### Observability Features
- **Distributed Tracing**: See request flows across services
- **Metrics Collection**: System and application metrics
- **Structured Logging**: Correlated logs with trace context
- **Health Monitoring**: Service health checks and alerts

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your Apps    │    │   devloop        │    │  OTel Collector │
│                │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Backend     │─┼────┼▶│ Rule: otel   │─┼────┼▶│ Receivers   │ │
│ │ Frontend    │ │    │ │ Rule: backend│ │    │ │ :4317 gRPC  │ │
│ │ Worker      │ │    │ │ Rule: frontend│ │   │ │ :4318 HTTP  │ │
│ └─────────────┘ │    │ │ Rule: worker │ │    │ └─────────────┘ │
│                 │    │ └──────────────┘ │    │                 │
│ Uses:           │    │                  │    │ Exports to:     │
│ OTEL_EXPORTER_  │    │ Manages:         │    │ - Console       │
│ OTLP_ENDPOINT   │    │ - File watching  │    │ - Files         │
│                 │    │ - Process mgmt   │    │ - Jaeger        │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Telemetry Data Flow

1. **Applications** send telemetry via OTLP to collector
2. **OTel Collector** receives, processes, and exports data
3. **devloop** manages both applications and collector lifecycle
4. **Unified View** of all telemetry in configured backends

## Accessing Telemetry

### Console Output
All telemetry appears in the devloop console with the `[otel-agent]` prefix.

### Health Checks
- Collector health: `curl http://localhost:13133`
- Collector metrics: `curl http://localhost:8888/metrics`

### Log Files
Persistent telemetry data saved to `./otel-logs/telemetry.jsonl`

## Customization

### Adding Exporters
Edit `../../otel-configs/devloop-default.yaml` to add exporters like:
- Jaeger for tracing
- Prometheus for metrics  
- External OTLP endpoints (Honeycomb, Datadog, etc.)

### Service Configuration
Each service can customize its telemetry via environment variables:
- `OTEL_SERVICE_NAME`: Service identifier
- `OTEL_RESOURCE_ATTRIBUTES`: Additional metadata
- `OTEL_EXPORTER_OTLP_ENDPOINT`: Collector endpoint

### Sampling and Filtering
Configure sampling rates and filtering in the OTel Collector config to manage data volume.

## Benefits

- **Unified Observability**: All services send telemetry to one place  
- **Development Ready**: Works out-of-the-box in dev environments  
- **Production Path**: Easy to extend configs for production  
- **Language Agnostic**: Works with any language that supports OTLP  
- **devloop Managed**: Collector lifecycle managed by devloop rules