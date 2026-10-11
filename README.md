# GKE Request Info Service

Application A for the GKE SRE assessment. It returns request, build, pod, cluster, and region metadata while exposing health, readiness, Prometheus metrics, controlled-error, and controlled-latency endpoints. Request completion logs are newline-delimited JSON with `severity`, `status_code`, `request_id`, and `latency_ms` fields for Cloud Logging, BigQuery, and Grafana analysis.

## Run locally

```bash
go test ./...
go run ./cmd/server
curl -s http://localhost:8080/ | jq
```

Endpoints: `/`, `/healthz`, `/readyz`, `/metrics`, `/error`, and `/delay?ms=250`.

Configuration: `PORT`, `APP_VERSION`, `CLUSTER_NAME`, `REGION`, `POD_NAME`,
optional `RESPONSE_SERVICE_URL`, `OBSERVABILITY_ENABLED`, and
`GOOGLE_CLOUD_PROJECT`.

Cloud observability is disabled by default for credential-free local runs. In
GKE, the service exports an inbound HTTP server span and an outbound client span
through authenticated OTLP, propagates W3C `traceparent` to the response
service, starts Cloud Profiler, correlates request logs with trace/span IDs, and
formats controlled errors for Error Reporting. Telemetry startup failures are
non-fatal. Kubernetes probe paths and Google load-balancer health checks are
excluded from tracing.

## Container and Kubernetes

```bash
docker build -t gke-request-info-service:local .
docker run --rm -p 8080:8080 gke-request-info-service:local
helm lint charts/gke-request-info-service
```

The image is distroless and non-root. The Helm chart uses numeric UID/GID `65532` so Kubernetes can verify the `runAsNonRoot` policy. It owns the Deployment, Service, ServiceAccount, HPA, and PodDisruptionBudget. CI/CD authentication will use GitHub OIDC and Google Workload Identity Federation; static service-account keys are prohibited.

For a release, set both the human-readable commit tag and the verified GAR
digest. The Deployment then uses the immutable `repository@sha256:...`
reference while `APP_VERSION` continues to report the commit tag:

```bash
helm upgrade request-info charts/gke-request-info-service \
  --namespace assessment-apps \
  --reuse-values \
  --set-string image.tag=COMMIT_SHA \
  --set-string image.digest=sha256:IMAGE_DIGEST
```

If `image.digest` is empty, the chart retains tag-based rendering for local
development. Production and Binary Authorization deployments must supply a
verified digest.

### Kubernetes configuration

The chart always creates a ConfigMap containing `APP_VERSION`, `CLUSTER_NAME`, and `REGION`. Set the deployment values without editing manifests:

```bash
helm upgrade --install request-info charts/gke-request-info-service \
  --set config.clusterName=gke-primary \
  --set config.region=us-central1 \
  --set config.responseServiceURL=http://response-gke-response-service.assessment-apps.svc.cluster.local \
  --set observability.enabled=true \
  --set observability.projectId=gke-sre-assesment
```

When `RESPONSE_SERVICE_URL` is configured, Application A calls Application B for each `GET /` request and forwards the same `X-Request-ID`. Its response includes the downstream metadata, HTTP status, and downstream latency. If Application B is unavailable, Application A returns `502 Bad Gateway`; the structured request log records the error severity and status code.

### External assessment endpoint

The response service remains private. To expose only the request service for a short assessment demonstration, enable the optional GKE Ingress:

```bash
helm upgrade request-info charts/gke-request-info-service \
  --namespace assessment-apps \
  --set ingress.enabled=true \
  --wait \
  --timeout 10m
```

This provisions a GKE external Application Load Balancer with an ephemeral public IP and a container-native NEG backend. The chart deliberately uses `kubernetes.io/ingress.class: gce`: GKE's built-in controller requires that annotation and does not process `spec.ingressClassName`, despite Kubernetes marking the annotation legacy. It is billable and HTTP-only until a domain, DNS record, and TLS certificate are deliberately configured. Disable it after collecting evidence with `helm upgrade request-info charts/gke-request-info-service --namespace assessment-apps --set ingress.enabled=false`.

### Multi-cluster backend export

After both clusters are registered to the same fleet and Multi-Cluster Services
is healthy, export the request Service from each cluster by setting
`serviceExport.enabled=true` on both Helm releases. The generated
`ServiceExport` has the same name and namespace as the chart's `Service`, so
GKE combines the healthy request-info Pods into one `ServiceImport` for a
multi-cluster Gateway backend.

```bash
helm upgrade request-info charts/gke-request-info-service \
  --namespace assessment-apps \
  --reuse-values \
  --set serviceExport.enabled=true \
  --wait \
  --timeout 10m \
  --rollback-on-failure
```

Export only the request service. The response service remains private and
cluster-local, preserving the flow `Gateway -> request-info -> local response`
and preventing direct external routing to Application B. Enabling the export
does not itself create a public address or load balancer; those are created
later by the platform-owned `Gateway` and `HTTPRoute`.

For sensitive settings, use an externally managed Kubernetes Secret by setting `secret.existingSecret`, or enable chart-managed creation with `secret.create=true` and a non-empty `secret.stringData` map. Never commit real secret values in a values file. The assessment's production design will use Secret Manager with Workload Identity Federation.
