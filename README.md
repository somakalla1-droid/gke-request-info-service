# GKE Request Info Service

Application A for the GKE SRE assessment. It returns request, build, pod, cluster, and region metadata while exposing health, readiness, Prometheus metrics, controlled-error, and controlled-latency endpoints.

## Run locally

```bash
go test ./...
go run ./cmd/server
curl -s http://localhost:8080/ | jq
```

Endpoints: `/`, `/healthz`, `/readyz`, `/metrics`, `/error`, and `/delay?ms=250`.

Configuration: `PORT`, `APP_VERSION`, `CLUSTER_NAME`, `REGION`, and `POD_NAME`.

## Container and Kubernetes

```bash
docker build -t gke-request-info-service:local .
docker run --rm -p 8080:8080 gke-request-info-service:local
helm lint charts/gke-request-info-service
```

The image is distroless and non-root. The Helm chart owns the Deployment, Service, ServiceAccount, HPA, and PodDisruptionBudget. CI/CD authentication will use GitHub OIDC and Google Workload Identity Federation; static service-account keys are prohibited.
