docker buildx build --platform linux/amd64,linux/arm64 -t kasbench/globeco-confirmation-service:latest --push .
kubectl rollout restart deployment globeco-confirmation-service   