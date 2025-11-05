docker buildx build --platform linux/amd64,linux/arm64 \
--tag kasbench/globeco-confirmation-service:latest \
--tag kasbench/globeco-confirmation-service:1.0.0 \
--file Dockerfile \
--push .
kubectl rollout restart deployment globeco-confirmation-service   