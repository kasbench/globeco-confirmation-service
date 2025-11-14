docker buildx build --platform linux/amd64,linux/arm64 \
--tag kasbench/globeco-confirmation-service:latest \
--tag kasbench/globeco-confirmation-service:1.0.3 \
--file Dockerfile \
--push .
kubectl delete -f k8s/deployment.yaml
kubectl apply -f k8s/deployment.yaml
# kubectl rollout restart deployment globeco-confirmation-service   