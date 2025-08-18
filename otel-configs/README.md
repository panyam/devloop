# OpenTelemetry Integration with Devloop

This directory contains OpenTelemetry Collector configurations optimized for use with devloop-managed processes.

## Quick Start

### 1. Install OpenTelemetry Collector

Choose your preferred installation method:

#### Option A: Download Binary
```bash
# Download the latest OTel Collector
curl -L -o otelcol \
  https://github.com/open-telemetry/opentelemetry-collector-releases/releases/latest/download/otelcol_linux_amd64

chmod +x otelcol
```

#### Option B: Using Package Manager
```bash
# macOS with Homebrew
brew install opentelemetry-collector

# Ubuntu/Debian
wget https://github.com/open-telemetry/opentelemetry-collector-releases/releases/latest/download/otelcol_0.91.0_linux_amd64.deb
sudo dpkg -i otelcol_0.91.0_linux_amd64.deb
```

#### Option C: Using Go
```bash
go install go.opentelemetry.io/collector/cmd/otelcol@latest
```

### 2. Add OTel Rule to Your Devloop Configuration

Add this rule to your `.devloop.yaml`:

```yaml
rules:
  - name: "otel-agent"
    watch:
      - action: "include"
        patterns: 
          - ".devloop.yaml"
          - "otel-configs/*.yaml"
    commands:
      - "otelcol --config=otel-configs/devloop-minimal.yaml"
    skip_run_on_init: false
```

### 3. Configure Your Applications

Set these environment variables in your application processes:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
export OTEL_SERVICE_NAME="my-service"
export OTEL_RESOURCE_ATTRIBUTES="service.version=1.0.0,deployment.environment=development"
```

## Available Configurations

### `devloop-minimal.yaml`
- **Use case**: Simple development setup
- **Features**: Basic OTLP receiver + console output
- **Best for**: Getting started, debugging

### `devloop-default.yaml`
- **Use case**: Full development environment
- **Features**: OTLP + host metrics, file export, health checks
- **Best for**: Local development with persistence

### `devloop-production-example.yaml`
- **Use case**: Production-like setup example
- **Features**: Jaeger, Prometheus, external OTLP exporters
- **Best for**: Testing production configurations locally

## Integration Examples

### Go Application
```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTracing() {
    exporter, _ := otlptracehttp.New(context.Background(),
        otlptracehttp.WithEndpoint("http://localhost:4318"),
        otlptracehttp.WithInsecure(),
    )
    
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
    )
    otel.SetTracerProvider(tp)
}
```

### Node.js Application
```javascript
const { NodeSDK } = require('@opentelemetry/sdk-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-otlp-http');

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter({
    url: 'http://localhost:4318/v1/traces',
  }),
  serviceName: 'my-node-service',
});

sdk.start();
```

### Python Application
```python
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

trace.set_tracer_provider(TracerProvider())
tracer = trace.get_tracer(__name__)

otlp_exporter = OTLPSpanExporter(endpoint="http://localhost:4318/v1/traces")
span_processor = BatchSpanProcessor(otlp_exporter)
trace.get_tracer_provider().add_span_processor(span_processor)
```

## Endpoints

When the OTel Collector is running, these endpoints are available:

- **OTLP gRPC**: `localhost:4317` (for gRPC clients)
- **OTLP HTTP**: `localhost:4318` (for HTTP clients)
- **Health Check**: `localhost:13133` (collector health)
- **Metrics**: `localhost:8888/metrics` (collector self-metrics)
- **pprof**: `localhost:1777` (performance profiling)

## Customization

To create your own configuration:

1. Copy one of the existing configs
2. Modify receivers, processors, and exporters as needed
3. Update your devloop rule to use the new config file
4. Restart devloop to pick up changes

## Troubleshooting

### Common Issues

1. **Port conflicts**: Change ports in the config if 4317/4318 are in use
2. **Permission errors**: Ensure otelcol binary has execute permissions
3. **Config errors**: Check logs for YAML syntax issues

### Useful Commands

```bash
# Test configuration
otelcol --config=otel-configs/devloop-minimal.yaml --dry-run

# Check health
curl http://localhost:13133

# View collector metrics
curl http://localhost:8888/metrics
```