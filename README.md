# Build the image
docker build -t vantage-exporter:latest .

# Run with environment variables
docker run -d \
  --name vantage-exporter \
  -p 8080:8080 \
  -e VANTAGE_BASE_URL="https://your-vantage-api.com" \
  -e VANTAGE_CLIENT_ID="your-client-id" \
  -e VANTAGE_CLIENT_SECRET="your-client-secret" \
  vantage-exporter:latest

# Test metrics endpoint
curl http://localhost:8080/metrics