# vantage-exporter

Prometheus exporter for ABBYY Vantage metrics

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

## Installing the Chart

Install with credentials:

```bash
helm install vantage-exporter ./helm \
  --set vantage.clientId="your-client-id" \
  --set vantage.clientSecret="your-secret"
```

Install with existing secret:

```bash
helm install vantage-exporter ./helm \
  --set vantage.existingSecret="vantage-credentials"
```

## Development Setup

### Option 1: Docker Compose (Simplest)

For quick local testing without Kubernetes:

```bash
cd docker-dev/
cp .env.example .env
# Edit .env with your credentials
docker-compose up -d
```

**Access services:**
- **Grafana**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **Exporter metrics**: http://localhost:8080/metrics

**Stop services:**
```bash
docker-compose down
```

**Develop dashboards:**
1. Create dashboards in Grafana (http://localhost:3000)
2. Export JSON (Dashboard settings → JSON Model)
3. Save to `helm/dashboards/your-dashboard.json`
4. Update `helm/templates/grafana/dashboard-configmap.yaml` to include new dashboard
5. Dashboards auto-load on restart

### Option 2: Kubernetes with Helm

For testing the Helm chart in a local cluster (kind/minikube):

**Initial setup:**
```bash
# Update chart dependencies
helm dependency update ./helm

# Install with dev values
helm install vantage-exporter ./helm \
  -f helm/values-dev.yaml \
  --set vantage.clientId="your-client-id" \
  --set vantage.clientSecret="your-secret"
```

**Access services:**
- **Grafana**: http://localhost:30300 (admin/admin)
- **Prometheus**: http://localhost:30090
- **Exporter metrics**: `kubectl port-forward svc/vantage-exporter-vantage-exporter 8080:8080`

**Develop dashboards:**
1. Port-forward Grafana: `kubectl port-forward svc/vantage-exporter-grafana 3000:80`
2. Access at http://localhost:3000 (admin/admin)
3. Create/edit dashboards
4. Export JSON and save to `helm/dashboards/`
5. Update ConfigMap if adding new dashboards
6. Upgrade release: `helm upgrade vantage-exporter ./helm -f helm/values-dev.yaml`

**The `values-dev.yaml` configuration:**
- Enables Grafana with pre-configured Prometheus datasource
- Enables Prometheus with optimized settings for development
- Disables persistence (no PVs required)
- Exposes services via NodePort for easy access
- Enables dashboard sidecar for auto-discovery

## Dashboard Development

To create or modify Grafana dashboards:

1. **Start Docker Compose environment:**
   ```bash
   cd docker-dev/
   docker-compose up -d
   ```

2. **Access Grafana** at http://localhost:3000 (admin/admin)
   - Prometheus datasource is pre-configured
   - Build your dashboards using vantage metrics

3. **Export dashboard JSON:**
   - Dashboard settings → JSON Model → Copy
   - Or: Share → Export → Save to file

4. **Save to version control:**
   ```bash
   # Save the exported JSON
   cat > helm/dashboards/my-dashboard.json
   # Paste JSON and Ctrl+D
   ```

5. **Update ConfigMap** if adding new dashboards:
   - Edit `helm/templates/grafana/dashboard-configmap.yaml`
   - Add new dashboard entry under `data:`

The dashboards will auto-load in both Docker Compose and Helm deployments.

## Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity for pod assignment |
| fullnameOverride | string | `""` | Override the full name of the release |
| grafana.enabled | bool | `false` | Enable Grafana installation |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"vantage-exporter"` | Container image repository |
| image.tag | string | `""` | Image tag (overrides the image tag whose default is the chart appVersion) |
| imagePullSecrets | list | `[]` | Secrets with credentials to pull images from a private registry |
| livenessProbe | object | `{"httpGet":{"path":"/metrics","port":"http"},"initialDelaySeconds":10,"periodSeconds":30,"timeoutSeconds":10}` | Liveness probe configuration |
| metricsPort | int | `8080` | Port on which the exporter exposes metrics |
| nameOverride | string | `""` | Override the name of the chart |
| nodeSelector | object | `{}` | Node selector for pod assignment |
| podAnnotations | object | `{"prometheus.io/path":"/metrics","prometheus.io/port":"8080","prometheus.io/scrape":"true"}` | Annotations to add to the pod |
| podSecurityContext | object | `{"fsGroup":1000,"runAsNonRoot":true,"runAsUser":1000}` | Security context for the pod |
| prometheus.enabled | bool | `false` | Enable Prometheus installation |
| readinessProbe | object | `{"httpGet":{"path":"/metrics","port":"http"},"initialDelaySeconds":5,"periodSeconds":10,"timeoutSeconds":5}` | Readiness probe configuration |
| replicaCount | int | `1` | Number of replicas for the vantage-exporter deployment |
| resources | object | `{"limits":{"cpu":"200m","memory":"128Mi"},"requests":{"cpu":"100m","memory":"64Mi"}}` | Resource limits and requests |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Security context for the container |
| service.annotations | object | `{}` | Annotations to add to the service |
| service.port | int | `8080` | Service port |
| service.type | string | `"ClusterIP"` | Service type |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | The name of the service account to use (if not set and create is true, a name is generated) |
| serviceMonitor.enabled | bool | `false` | Create a ServiceMonitor for Prometheus Operator |
| serviceMonitor.interval | string | `"30s"` | Scrape interval |
| serviceMonitor.labels | object | `{}` | Additional labels for ServiceMonitor |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout |
| tolerations | list | `[]` | Tolerations for pod assignment |
| vantage.baseUrl | string | `"https://vantage-us.abbyy.com"` | Vantage API base URL |
| vantage.clientId | string | `""` | Vantage API client ID |
| vantage.clientSecret | string | `""` | Vantage API client secret |
| vantage.existingSecret | string | `""` | Name of existing secret to use for credentials (instead of creating one) |
| vantage.existingSecretKeys.clientId | string | `"client-id"` | Key in existing secret containing the client ID |
| vantage.existingSecretKeys.clientSecret | string | `"client-secret"` | Key in existing secret containing the client secret |

## Regenerating Documentation

This README is generated from the Helm chart values and templates. To regenerate:

```bash
# Install helm-docs if not already installed
go install github.com/norwoodj/helm-docs/cmd/helm-docs@latest

# Regenerate the README
cd helm/
helm-docs --output-file=../README.md
```

The documentation is generated from:
- `helm/Chart.yaml` - Chart metadata
- `helm/values.yaml` - Configuration values and comments
- `helm/README.md.gotmpl` - README template

