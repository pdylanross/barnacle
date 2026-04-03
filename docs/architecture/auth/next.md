# Authentication - Next

## Goals

Replace the current hardcoded-credentials-per-upstream model (see [current.md](current.md)) with two new auth modes that cover real-world deployment scenarios. Each upstream is configured for exactly one mode.

## Auth Modes

### 1. Passthrough Auth

Barnacle acts as a transparent proxy for authentication. The client provides credentials (via `Authorization` header), and barnacle forwards them directly to the upstream registry.

**Behavior:**
- Client sends `Authorization: Basic ...` or `Authorization: Bearer ...` with every request
- Barnacle extracts the header and passes it through to the upstream
- If the client sends no credentials, the upstream request is made without auth (anonymous)
- If the upstream returns `401`, barnacle returns `401` to the client with the upstream's `WWW-Authenticate` header forwarded back
- Barnacle does not validate, inspect, or cache credentials — it's a pass-through pipe

**Use cases:**
- Developers pulling through barnacle with their own registry credentials
- CI systems with per-job tokens
- Environments where barnacle shouldn't store any secrets

**Config:**
```yaml
upstreams:
  dockerhub:
    registry: registry-1.docker.io
    authentication:
      mode: passthrough
```

### 2. Environment Auth

Barnacle discovers credentials from its runtime environment. Clients never provide credentials — barnacle handles upstream auth entirely.

**Credential sources (resolved in priority order):**

| Source | Description | Env/Path |
|--------|-------------|----------|
| Static config | Current behavior — explicit username/password/token in config | YAML config |
| Docker config | Standard `~/.docker/config.json` or `DOCKER_CONFIG` | `$DOCKER_CONFIG/config.json`, `~/.docker/config.json` |
| K8s ImagePullSecrets | Mounted secrets in pod | `/var/run/secrets/...` or API lookup |
| GCR / Artifact Registry | GCP workload identity, metadata server, `GOOGLE_APPLICATION_CREDENTIALS` | GCE metadata, service account key |
| ECR | AWS IAM role, instance profile, `AWS_*` env vars | STS + ECR `GetAuthorizationToken` |
| ACR | Azure managed identity, `AZURE_*` env vars | IMDS + ACR token exchange |

**Resolution strategy:**
- At startup (and periodically for expiring tokens), barnacle resolves credentials for each environment-auth upstream
- The upstream's `registry` hostname is used to match against credential sources (e.g., `*.gcr.io` → try GCR provider, `*.dkr.ecr.*.amazonaws.com` → try ECR provider)
- If multiple sources match, use the most specific one (explicit config > dockercfg > cloud provider)
- Credentials that expire (ECR tokens, GCR access tokens) are refreshed automatically

**Config:**
```yaml
upstreams:
  # Minimal — auto-detect credentials from environment
  gcr-prod:
    registry: us-docker.pkg.dev
    authentication:
      mode: environment

  # Explicit static credentials (backwards compatible with current)
  private-reg:
    registry: registry.internal.corp
    authentication:
      mode: environment
      static:
        username: svc-barnacle
        password: ${REGISTRY_PASSWORD}

  # Hint which cloud provider to use (skip auto-detection)
  ecr:
    registry: 123456789.dkr.ecr.us-east-1.amazonaws.com
    authentication:
      mode: environment
      provider: ecr
```

**Client-facing auth:** None. Clients pull from barnacle without credentials. Barnacle is the trusted party.

## Mode Comparison

| Aspect | Passthrough | Environment |
|--------|-------------|-------------|
| Who provides upstream creds? | Client | Barnacle |
| Client sends `Authorization`? | Yes | No |
| Barnacle stores secrets? | No | Yes (or discovers them) |
| Barnacle returns `401`? | Yes (forwarded from upstream) | No (barnacle always has creds) |
| Token refresh? | N/A (client's problem) | Barnacle handles it |
| Cache keying | Must include client identity | Shared across all clients |

## Cache Implications

**Passthrough:** Cache is shared across all clients, but every cache hit performs a HEAD request to the upstream with the client's credentials before serving content. This ensures the client is authorized to access the content even when served from cache. The added latency is acceptable — it mirrors how CDNs handle `must-revalidate` and ensures barnacle never serves cached content to unauthorized clients.

If the HEAD check returns `401` or `403`, barnacle returns the upstream's error response to the client. If the HEAD check succeeds, barnacle serves from cache without re-downloading.

**Environment:** Cache is shared across all clients. No per-client scoping or revalidation needed since barnacle owns the credentials and all clients are treated equally.

## Configuration schema design

Auth settings are configured using a flat, named-key structure where each supported auth method has a dedicated top-level field. Only one method should be set at a time; this is enforced at validation rather than at the schema level.

```yaml
auth:
    basic:
        username: xxx
        password: xxx
```

This approach keeps the schema straightforward and self-documenting — valid fields for each auth method are fully described without requiring a polymorphic type/spec envelope. Mutual exclusivity is validated explicitly on startup, failing fast with a clear error if misconfigured.

## Out of Scope

- Client-facing auth enforcement (no plans to require clients to authenticate to barnacle itself)
- Per-repository access control
- Credential caching/sharing across upstreams
- Push support (barnacle is read-only / pull-through)
- Backwards compatibility with current auth config format