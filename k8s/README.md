# Kubernetes Manifests for globeco-confirmation-service

This directory contains Kubernetes manifests for deploying the `globeco-confirmation-service` microservice in the `globeco` namespace.

## Files

- `deployment.yaml`: Deployment for the service
- `service.yaml`: ClusterIP Service exposing port 8080

## Features

- **Liveness, readiness, and startup probes** for robust health checking
  - Liveness probe: `/healthz` with a 240s timeout
  - Readiness probe: `/readyz`
  - Startup probe: `/healthz`
- **Resource limits**: 100 millicores CPU, 200Mi memory per pod
- **Scalability**: Starts with 1 replica, can scale up to 100 via HPA
- **Service**: Exposes port 8080 as a ClusterIP

## Usage

1. Update the `image` field in `deployment.yaml` with your built image.
2. Apply the manifests:

   ```sh
   kubectl apply -f k8s/deployment.yaml
   kubectl apply -f k8s/service.yaml
   ```

3. The service will be available within the cluster at `globeco-confirmation-service.globeco.svc.cluster.local:8080`.

## Health Endpoints
- `/healthz` for liveness and startup
- `/readyz` for readiness

## Notes
- Namespace `globeco` must exist before applying these manifests.
- The deployment is configured for multi-architecture Docker images (see project Dockerfile). 