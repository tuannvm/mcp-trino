# mcp-trino Helm Chart Installation Guide

This guide provides step-by-step instructions for installing the mcp-trino Helm chart on Amazon EKS.

> **OAuth Authentication**: mcp-trino uses [oauth-mcp-proxy](https://github.com/tuannvm/oauth-mcp-proxy) for OAuth 2.1 authentication. See the library documentation for detailed provider configuration and security best practices.

## Quick Start

### 1. Prerequisites

- Kubernetes cluster (EKS recommended)
- Helm 3.0+ installed
- kubectl configured for your cluster
- AWS Load Balancer Controller (for EKS ingress)

### 2. Basic Installation

```bash
# Clone the repository
git clone https://github.com/tuannvm/mcp-trino.git
cd mcp-trino

# Install with default values
helm install mcp-trino ./charts/mcp-trino

# Check deployment status
kubectl get pods -l app.kubernetes.io/name=mcp-trino
```

### 3. Development Installation

```bash
# Install with development values
helm install mcp-trino ./charts/mcp-trino -f ./charts/mcp-trino/values-development.yaml

# Port forward to test locally
kubectl port-forward svc/mcp-trino 8080:8080

# Test the MCP server
curl http://localhost:8080/
```

### 4. Production Installation on EKS

```bash
# Copy and customize production values
cp ./charts/mcp-trino/values-production.yaml my-production-values.yaml

# Edit the values file with your specific configuration
# - Update trino.host to your Trino server
# - Configure OAuth settings if needed
# - Set appropriate resource limits
# - Configure IRSA role ARN

# Install with production configuration
helm install mcp-trino ./charts/mcp-trino -f my-production-values.yaml

# Verify installation
helm test mcp-trino
```

## Configuration Examples

### Basic Trino Connection

```yaml
# values.yaml
trino:
  host: "my-trino.company.com"
  port: 8080
  user: "analytics-user"
  catalog: "hive"
  schema: "analytics"
```

### OAuth with Okta - Fixed Redirect Mode (Development)

```yaml
# values.yaml - Development with localhost support
trino:
  oauth:
    enabled: true
    mode: "proxy"
    provider: "okta"
    jwtSecret: ""  # Generate with: openssl rand -hex 32
    redirectURIs: "https://mcp-server.company.com/oauth/callback"  # Fixed mode
    oidc:
      issuer: "https://company.okta.com"
      audience: "trino-mcp"
      clientId: "mcp-client-id"

# Install with secrets
helm install mcp-trino ./charts/mcp-trino \
  -f values.yaml \
  --set trino.oauth.jwtSecret="$(openssl rand -hex 32)" \
  --set trino.oauth.oidc.clientSecret="your-client-secret"
```

### OAuth with Okta - Allowlist Mode (Production)

```yaml
# values.yaml - Production with allowlist
trino:
  oauth:
    enabled: true
    mode: "proxy"
    provider: "okta"
    jwtSecret: ""  # Must be same across all pods
    redirectURIs: "https://app1.company.com/callback,https://app2.company.com/callback"  # Allowlist mode
    oidc:
      issuer: "https://company.okta.com"
      audience: "https://api.company.com"
      clientId: "production-client-id"

# Install with secrets
helm install mcp-trino ./charts/mcp-trino \
  -f values.yaml \
  --set trino.oauth.jwtSecret="your-persistent-jwt-secret" \
  --set trino.oauth.oidc.clientSecret="your-client-secret"
```

### OAuth Native Mode (Zero Server-Side Secrets)

```yaml
# values.yaml - Most secure, client handles OAuth
trino:
  oauth:
    enabled: true
    mode: "native"
    provider: "okta"
    oidc:
      issuer: "https://company.okta.com"
      audience: "https://mcp-server.com"
      # No clientId or clientSecret needed

# Install without secrets
helm install mcp-trino ./charts/mcp-trino -f values.yaml
```

### EKS with Load Balancer

```yaml
# values.yaml
service:
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
    service.beta.kubernetes.io/aws-load-balancer-scheme: internal

eks:
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/mcp-trino-role
```

## Common Operations

### Upgrading

```bash
# Upgrade to new version
helm upgrade mcp-trino ./charts/mcp-trino --reuse-values

# Upgrade with new configuration
helm upgrade mcp-trino ./charts/mcp-trino -f my-values.yaml
```

### Monitoring

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=mcp-trino

# View logs
kubectl logs -l app.kubernetes.io/name=mcp-trino

# Check service endpoints
kubectl get svc mcp-trino

# Test connectivity
kubectl run curl --image=curlimages/curl -i --tty --rm -- \
  curl -f http://mcp-trino:8080/
```

### Troubleshooting

```bash
# Check pod events
kubectl describe pod -l app.kubernetes.io/name=mcp-trino

# Check service
kubectl describe svc mcp-trino

# Check ingress (if enabled)
kubectl describe ingress mcp-trino

# View configuration
kubectl get configmap mcp-trino -o yaml
kubectl get secret mcp-trino -o yaml
```

### Scaling

```bash
# Manual scaling
kubectl scale deployment mcp-trino --replicas=3

# Enable autoscaling via values
helm upgrade mcp-trino ./charts/mcp-trino \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10
```

## Security Considerations

### Pod Security

The chart implements security best practices:
- Non-root user (UID 65534)
- Read-only root filesystem
- Dropped capabilities
- No privilege escalation

### OAuth Security

**Critical for Multi-Pod Deployments:**

⚠️ **JWT_SECRET must be configured** when running multiple replicas to ensure state signing consistency:

```bash
# Generate secure JWT secret
export JWT_SECRET=$(openssl rand -hex 32)

helm install mcp-trino ./charts/mcp-trino \
  --set trino.oauth.jwtSecret="$JWT_SECRET"
```

**Redirect URI Modes:**

- **Fixed Mode** (single URI): Only accepts localhost callbacks (development)
- **Allowlist Mode** (comma-separated): Exact match required (production)
- See [OAuth Architecture](../../docs/oauth.md) for security details

### Network Policies

Enable network policies for production:

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ai-services
```

### Secrets Management

For production, use external secret management:

```yaml
# Use AWS Secrets Manager
extraEnvVarsSecret: "mcp-trino-secrets"

# Or use sealed secrets
trino:
  password: ""  # Leave empty, provide via sealed secret
  oauth:
    jwtSecret: ""  # Provide via sealed secret
    oidc:
      clientSecret: ""  # Provide via sealed secret
```

## Performance Tuning

### Resource Allocation

```yaml
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi
```

### Connection Pooling

Trino client uses connection pooling by default:
- Max open connections: 10
- Max idle connections: 5
- Connection max lifetime: 5 minutes

### Query Timeout

```yaml
trino:
  queryTimeout: 60  # seconds
```

## AWS EKS Specific Setup

### IAM Role for Service Account (IRSA)

1. Create IAM role:
```bash
eksctl create iamserviceaccount \
  --name mcp-trino \
  --namespace default \
  --cluster my-cluster \
  --attach-policy-arn arn:aws:iam::aws:policy/CloudWatchLogsFullAccess \
  --approve
```

2. Update values:
```yaml
eks:
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/eksctl-cluster-addon-iamserviceaccount-Role
```

### Load Balancer Controller

Ensure AWS Load Balancer Controller is installed:

```bash
helm repo add eks https://aws.github.io/eks-charts
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=my-cluster
```

## Cleanup

```bash
# Uninstall the chart
helm uninstall mcp-trino

# Clean up test resources
kubectl delete pod --selector=helm.sh/hook=test
```

## Next Steps

- Configure monitoring with Prometheus
- Set up log aggregation with FluentBit
- Implement backup strategies for configuration
- Set up multi-region deployment for HA
- Configure custom domains with Route53

For more advanced configuration options, see the [README.md](mcp-trino/README.md).