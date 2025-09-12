# Build the image
docker build -t vantage-exporter:latest .

# Run with environment variables
docker run -d \
  --name vantage-exporter \
  -p 8080:8080 \
  -e VANTAGE_API_URL="https://your-vantage-api.com" \
  -e VANTAGE_API_TOKEN="your-token" \
  vantage-exporter:latest

# Test metrics endpoint
curl http://localhost:8080/metrics