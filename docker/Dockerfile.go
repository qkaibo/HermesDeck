FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY src/go/go.mod src/go/go.sum ./
RUN go mod download
COPY src/go/ ./
RUN CGO_ENABLED=0 go build -o /hermesdeck ./cmd/hermesdeck/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /hermesdeck /app/hermesdeck
EXPOSE 8080 50051
ENTRYPOINT ["/app/hermesdeck"]
