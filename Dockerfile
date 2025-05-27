# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
WORKDIR /src/cmd/fix-engine
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o /out/globeco-fix-engine

FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/globeco-fix-engine /globeco-fix-engine
# COPY --from=builder /src/migrations /migrations
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/globeco-fix-engine"] 