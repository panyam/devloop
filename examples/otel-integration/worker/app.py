#!/usr/bin/env python3

import time
import random
import logging
import requests
from datetime import datetime

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.instrumentation.requests import RequestsInstrumentation

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

def init_tracing():
    """Initialize OpenTelemetry tracing"""
    resource = Resource.create({
        "service.name": "worker-service",
        "service.version": "1.0.0",
    })
    
    trace.set_tracer_provider(TracerProvider(resource=resource))
    tracer_provider = trace.get_tracer_provider()
    
    # OTLP exporter
    otlp_exporter = OTLPSpanExporter(endpoint="http://localhost:4318/v1/traces")
    span_processor = BatchSpanProcessor(otlp_exporter)
    tracer_provider.add_span_processor(span_processor)
    
    # Auto-instrument requests
    RequestsInstrumentation().instrument()
    
    logger.info("OpenTelemetry tracing initialized")

def check_services():
    """Check health of other services"""
    tracer = trace.get_tracer(__name__)
    
    with tracer.start_as_current_span("check_services") as span:
        services = [
            ("backend", "http://localhost:8080/health"),
            ("frontend", "http://localhost:3000/health")
        ]
        
        results = {}
        for service_name, url in services:
            with tracer.start_as_current_span(f"check_{service_name}") as service_span:
                try:
                    response = requests.get(url, timeout=2)
                    if response.status_code == 200:
                        results[service_name] = "healthy"
                        logger.info(f"{service_name} is healthy")
                    else:
                        results[service_name] = f"unhealthy (status: {response.status_code})"
                        logger.warning(f"{service_name} returned status {response.status_code}")
                except requests.exceptions.RequestException as e:
                    results[service_name] = f"error: {str(e)}"
                    logger.error(f"Error checking {service_name}: {e}")
                    service_span.set_status(trace.Status(trace.StatusCode.ERROR, str(e)))
        
        span.set_attributes({
            "services.total": len(services),
            "services.healthy": sum(1 for status in results.values() if status == "healthy")
        })
        
        return results

def simulate_work():
    """Simulate some background work"""
    tracer = trace.get_tracer(__name__)
    
    with tracer.start_as_current_span("simulate_work") as span:
        work_duration = random.uniform(0.1, 2.0)
        work_type = random.choice(["data_processing", "cache_update", "cleanup", "sync"])
        
        span.set_attributes({
            "work.type": work_type,
            "work.duration_seconds": work_duration
        })
        
        logger.info(f"Starting {work_type} (duration: {work_duration:.2f}s)")
        time.sleep(work_duration)
        logger.info(f"Completed {work_type}")

def main():
    """Main worker loop"""
    init_tracing()
    tracer = trace.get_tracer(__name__)
    
    logger.info("Worker service started")
    
    iteration = 0
    while True:
        iteration += 1
        
        with tracer.start_as_current_span("worker_iteration") as span:
            span.set_attributes({
                "iteration.number": iteration,
                "iteration.timestamp": datetime.now().isoformat()
            })
            
            logger.info(f"Worker iteration {iteration}")
            
            # Check services every 5 iterations
            if iteration % 5 == 0:
                service_status = check_services()
                logger.info(f"Service status: {service_status}")
            
            # Simulate work
            simulate_work()
            
            # Wait before next iteration
            wait_time = random.uniform(3, 8)
            logger.info(f"Waiting {wait_time:.1f} seconds before next iteration")
            time.sleep(wait_time)

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        logger.info("Worker service stopped by user")
    except Exception as e:
        logger.error(f"Worker service error: {e}")
        raise