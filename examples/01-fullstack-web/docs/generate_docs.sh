#!/bin/bash

# Simple documentation generator
echo "[docs] Generating API documentation..."

# Copy template and replace timestamp
cp docs/api_template.md docs/api.md
sed -i.bak "s/{{TIMESTAMP}}/$(date '+%Y-%m-%d %H:%M:%S')/g" docs/api.md
rm -f docs/api.md.bak

echo "[docs] API documentation generated at docs/api.md"