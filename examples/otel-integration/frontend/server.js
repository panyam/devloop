const { NodeSDK } = require('@opentelemetry/sdk-node');
const { OTLPTraceExporter } = require('@opentelemetry/exporter-otlp-http');
const { ExpressInstrumentation } = require('@opentelemetry/instrumentation-express');
const { HttpInstrumentation } = require('@opentelemetry/instrumentation-http');

// Initialize OpenTelemetry
const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter({
    url: 'http://localhost:4318/v1/traces',
  }),
  instrumentations: [
    new HttpInstrumentation(),
    new ExpressInstrumentation(),
  ],
  serviceName: 'frontend-service',
  serviceVersion: '1.0.0',
});

sdk.start();
console.log('OpenTelemetry tracing initialized');

const express = require('express');
const http = require('http');

const app = express();
const port = 3000;

// Middleware for parsing JSON
app.use(express.json());

// Serve static files
app.use(express.static('public'));

// Home route
app.get('/', (req, res) => {
  console.log('Frontend: Serving home page');
  res.send(`
    <html>
      <head><title>Frontend Service</title></head>
      <body>
        <h1>Frontend Service</h1>
        <p>Current time: ${new Date().toISOString()}</p>
        <button onclick="callBackend()">Call Backend</button>
        <div id="result"></div>
        
        <script>
          async function callBackend() {
            try {
              const response = await fetch('http://localhost:8080/');
              const text = await response.text();
              document.getElementById('result').innerHTML = '<pre>' + text + '</pre>';
            } catch (error) {
              document.getElementById('result').innerHTML = 'Error: ' + error.message;
            }
          }
        </script>
      </body>
    </html>
  `);
});

// API route that calls backend
app.get('/api/backend', async (req, res) => {
  try {
    console.log('Frontend: Making request to backend');
    
    const backendResponse = await new Promise((resolve, reject) => {
      const request = http.get('http://localhost:8080/', (response) => {
        let data = '';
        response.on('data', chunk => data += chunk);
        response.on('end', () => resolve(data));
      });
      request.on('error', reject);
      request.setTimeout(5000, () => {
        request.destroy();
        reject(new Error('Request timeout'));
      });
    });
    
    res.json({
      frontend_timestamp: new Date().toISOString(),
      backend_response: backendResponse.trim()
    });
  } catch (error) {
    console.error('Frontend: Error calling backend:', error.message);
    res.status(500).json({ error: error.message });
  }
});

// Health check
app.get('/health', (req, res) => {
  res.json({ 
    status: 'healthy', 
    service: 'frontend-service',
    timestamp: new Date().toISOString() 
  });
});

app.listen(port, () => {
  console.log(`Frontend service listening on port ${port}`);
  console.log(`Visit http://localhost:${port} to see the frontend`);
});