# mcp-trino Helm Chart

A Helm chart for deploying mcp-trino as a remote MCP server on Kubernetes, specifically optimized for Amazon EKS.

## Description

mcp-trino is a Model Context Protocol (MCP) server that enables AI assistants to interact with Trino's distributed SQL query engine. This Helm chart provides a production-ready deployment solution with comprehensive security, scalability, and AWS integration features.

**OAuth Authentication**: mcp-trino uses [oauth-mcp-proxy](https://github.com/tuannvm/oauth-mcp-proxy) for OAuth 2.1 authentication. See the [oauth-mcp-proxy documentation](https://github.com/tuannvm/oauth-mcp-proxy#readme) for detailed provider setup and security best practices.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- For EKS: AWS Load Balancer Controller (for Ingress)
- For OAuth: Configured OIDC provider

## Installing the Chart

### Basic Installation

```bash
helm repo add mcp-trino https://tuannvm.github.io/mcp-trino-helm
helm install my-mcp-trino mcp-trino/mcp-trino
```

### Custom Configuration

```bash
helm install my-mcp-trino mcp-trino/mcp-trino \
  --set trino.host=my-trino.example.com \
  --set trino.user=analytics-user \
  --set service.type=LoadBalancer
```

### Production EKS Deployment

```bash
# Create production-values.yaml
cat <<EOF > production-values.yaml
replicaCount: 3

service:
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
    service.beta.kubernetes.io/aws-load-balancer-scheme: internal

eks:
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/mcp-trino-role

trino:
  host: "production-trino.company.internal"
  oauth:
    enabled: true
    provider: "okta"
    oidc:
      issuer: "https://company.okta.com"
      clientId: "mcp-trino-client"

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10

networkPolicy:
  enabled: true
EOF

helm install mcp-trino mcp-trino/mcp-trino -f production-values.yaml
```

## Configuration

The following table lists the configurable parameters and their default values.

### Global Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Global image registry | `""` |
| `global.imagePullSecrets` | Global image pull secrets | `[]` |

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Image registry | `ghcr.io` |
| `image.repository` | Image repository | `tuannvm/mcp-trino` |
| `image.tag` | Image tag (uses Chart.appVersion if empty) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Deployment Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `strategy.type` | Deployment strategy | `RollingUpdate` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `service.targetPort` | Target port | `8080` |

### MCP Server Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mcpServer.transport` | Transport protocol (http/stdio) | `http` |
| `mcpServer.port` | Server port | `8080` |
| `mcpServer.host` | Server host | `0.0.0.0` |

### Trino Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `trino.host` | Trino server host | `trino` |
| `trino.port` | Trino server port | `8080` |
| `trino.user` | Trino user | `trino` |
| `trino.password` | Trino password | `""` |
| `trino.catalog` | Default catalog | `memory` |
| `trino.schema` | Default schema | `default` |
| `trino.scheme` | Connection scheme (http/https) | `https` |
| `trino.ssl` | Enable SSL | `true` |
| `trino.sslInsecure` | Allow insecure SSL | `false` |
| `trino.allowWriteQueries` | Allow write queries | `false` |
| `trino.queryTimeout` | Query timeout (seconds) | `30` |

### OAuth Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `trino.oauth.enabled` | Enable OAuth | `false` |
| `trino.oauth.provider` | OAuth provider (hmac/okta/google/azure) | `hmac` |
| `trino.oauth.jwtSecret` | JWT secret for HMAC provider | `""` |
| `trino.oauth.redirectURI` | OAuth redirect URI | `""` |
| `trino.oauth.oidc.issuer` | OIDC issuer URL | `""` |
| `trino.oauth.oidc.audience` | OIDC audience | `""` |
| `trino.oauth.oidc.clientId` | OIDC client ID | `""` |
| `trino.oauth.oidc.clientSecret` | OIDC client secret | `""` |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Autoscaling Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `10` |
| `autoscaling.targetCPUUtilizationPercentage` | CPU target | `80` |
| `autoscaling.targetMemoryUtilizationPercentage` | Memory target | `80` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.runAsNonRoot` | Run as non-root | `true` |
| `podSecurityContext.runAsUser` | User ID | `65534` |
| `podSecurityContext.runAsGroup` | Group ID | `65534` |
| `podSecurityContext.fsGroup` | FS Group ID | `65534` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `securityContext.readOnlyRootFilesystem` | Read-only root filesystem | `true` |

### EKS Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `eks.loadBalancer.enabled` | Enable AWS Load Balancer | `false` |
| `eks.serviceAccount.annotations` | IRSA annotations | `{}` |

### Network Policy

| Parameter | Description | Default |
|-----------|-------------|---------|
| `networkPolicy.enabled` | Enable NetworkPolicy | `false` |

## Examples

### Basic Trino Connection

```yaml
trino:
  host: "trino.company.internal"
  port: 8080
  user: "analytics"
  catalog: "hive"
  schema: "default"
```

### OAuth with Okta

```yaml
trino:
  oauth:
    enabled: true
    provider: "okta"
    oidc:
      issuer: "https://company.okta.com"
      audience: "trino-mcp"
      clientId: "mcp-client-id"
      clientSecret: "secret-value"
```

### EKS with IRSA

```yaml
eks:
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/mcp-trino-role

service:
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
    service.beta.kubernetes.io/aws-load-balancer-scheme: internal
```

### High Availability Setup

```yaml
replicaCount: 3

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10

podDisruptionBudget:
  enabled: true
  minAvailable: 1

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - mcp-trino
        topologyKey: kubernetes.io/hostname
      weight: 100
```

## Testing

Run the included tests:

```bash
helm test my-mcp-trino
```

Test the MCP server manually:

```bash
kubectl run curl --image=curlimages/curl -i --tty --rm -- curl -f http://my-mcp-trino:8080/
```

## Upgrading

```bash
helm upgrade my-mcp-trino mcp-trino/mcp-trino --reuse-values --set image.tag=v0.3.0
```

## Uninstalling

```bash
helm uninstall my-mcp-trino
```

## Development

### Linting

```bash
helm lint charts/mcp-trino
```

### Template Rendering

```bash
helm template my-mcp-trino charts/mcp-trino --debug
```

### Dry Run

```bash
helm install --dry-run --debug my-mcp-trino charts/mcp-trino
```

## Support

- **Documentation**: https://github.com/tuannvm/mcp-trino
- **Issues**: https://github.com/tuannvm/mcp-trino/issues
- **Docker Images**: https://github.com/tuannvm/mcp-trino/pkgs/container/mcp-trino

## License

This chart is licensed under the MIT License.