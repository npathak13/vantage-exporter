FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o vantage-exporter main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/vantage-exporter .
EXPOSE 8080
CMD ["./vantage-exporter"]