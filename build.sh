docker buildx build --platform linux/amd64,linux/arm64 \
 -t kasbench/globeco-confirmation-service:latest \
-t kasbench/globeco-confirmation-service:1.0.0 \
 --push .