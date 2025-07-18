#!/bin/bash
set -e

NAMESPACE=globeco


# Deploy application Deployment and Service
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
echo "Deployment complete."
