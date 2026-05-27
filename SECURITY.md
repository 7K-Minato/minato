# Security Policy

## Supported Versions

The following versions of Minato are currently supported with security updates:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |
| < 0.1   | :x:                |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please follow these steps:

### Responsible Disclosure

1. **Do not** open a public issue
2. Email security concerns to: security@7k-group.dev (or project maintainers)
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix timeline**: Depends on severity
  - Critical: 7 days
  - High: 30 days
  - Medium: 90 days
  - Low: Next release

### Disclosure Process

1. We will acknowledge receipt of your report
2. We will investigate and validate the vulnerability
3. We will develop and test a fix
4. We will coordinate disclosure with you
5. We will release the fix and publicly disclose

## Security Best Practices

### For Operators

- Run Minato with PSS:restricted pod security standards
- Use NetworkPolicies to restrict traffic
- Enable RBAC with least privilege
- Regularly update to latest versions
- Monitor audit logs
- Use TLS for all communications

### For Developers

- Never commit secrets or credentials
- Use distroless/minimal base images
- Pin dependency versions
- Run security scans in CI
- Follow secure coding practices

## Security Features

Minato includes several security features:

- **Non-root containers**: All containers run as non-root (UID 65532)
- **Distroless images**: Minimal attack surface
- **RBAC**: Fine-grained Kubernetes RBAC
- **Network Policies**: Traffic isolation between tenants
- **Audit Trail**: ActionExecution provides audit logging
- **PSS Restricted**: Compatible with Pod Security Standards

## Known Security Considerations

### WebSocket Console

The WebSocket console endpoint supports configurable origin checking. By default, all origins are allowed for development. In production:

```yaml
# Set allowed origins
allowedOrigins:
  - "https://your-domain.com"
```

### gRPC Communication

Agent gRPC connections use plaintext by default within the cluster. For production:
- Consider enabling mTLS between operator and agents
- Use service mesh for encrypted pod-to-pod communication

### Control Plane API

The control plane currently uses basic authentication. For production deployments:
- Deploy behind an API gateway with OIDC/OAuth2
- Enable TLS termination at the ingress
- Implement rate limiting

## Security Scanning

We regularly perform:

- `govulncheck` for Go vulnerability scanning
- Trivy for container image scanning
- Dependabot for dependency updates
- CodeQL for static analysis

## Acknowledgments

We thank the following individuals for responsibly disclosing security issues:

- *None yet - be the first!*
