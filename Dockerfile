# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
WORKDIR /src/cmd/confirmation-service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o /out/globeco-confirmation-service

FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/globeco-confirmation-service /globeco-confirmation-service
COPY --from=builder /src/config.yaml /config.yaml
# COPY --from=builder /src/migrations /migrations
EXPOSE 8086
USER nonroot
ENTRYPOINT ["/globeco-confirmation-service"] 