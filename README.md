# GKE Request Info Service

Application A for the GKE SRE assessment. It returns request, build, pod, cluster, and region metadata while exposing health, readiness, Prometheus metrics, controlled-error, and controlled-latency endpoints. Request completion logs are newline-delimited JSON with `severity`, `status_code`, `request_id`, and `latency_ms` fields for Cloud Logging, BigQuery, and Grafana analysis.

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

### Kubernetes configuration

The chart always creates a ConfigMap containing `APP_VERSION`, `CLUSTER_NAME`, and `REGION`. Set the deployment values without editing manifests:

```bash
helm upgrade --install request-info charts/gke-request-info-service \
  --set config.clusterName=gke-primary \
  --set config.region=us-central1
```

For sensitive settings, use an externally managed Kubernetes Secret by setting `secret.existingSecret`, or enable chart-managed creation with `secret.create=true` and a non-empty `secret.stringData` map. Never commit real secret values in a values file. The assessment's production design will use Secret Manager with Workload Identity Federation.
