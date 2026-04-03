# Authentication - Current

## Overview

Barnacle has two authentication surfaces:

1. **Upstream auth** — how barnacle authenticates to upstream registries when pulling content
2. **Client-facing auth** — how clients (Docker, containerd, kubelet) authenticate to barnacle

Currently, only upstream auth is implemented. Client-facing auth is entirely absent.

## Upstream Authentication

Each upstream registry in the config supports one of three auth types:

| Type | Config Fields | Implementation |
|------|--------------|----------------|
| Anonymous | *(none — default)* | No credentials sent |
| Basic | `username`, `password` | HTTP Basic Auth via `go-containerregistry` `authn.FromConfig` |
| Bearer | `token` | Registry token via `go-containerregistry` `authn.FromConfig` |

### Configuration

```yaml
upstreams:
  dockerhub:
    registry: registry-1.docker.io
    authentication:
      basic:
        username: myuser
        password: mypassword
  ghcr:
    registry: ghcr.io
    authentication:
      bearer:
        token: ghp_xxxxxxxxxxxx
  local:
    registry: localhost:5000
    # defaults to anonymous
```

Validation enforces exactly one auth type per upstream (`pkg/configuration/upstream.go:48-85`). Credentials are static — loaded from YAML config at startup with envsubst support for secret injection.

### Code Path

1. Config loaded → `UpstreamConfiguration.Authentication` populated
2. `internal/registry/factory.go:buildUpstream()` passes config to `standard.New()`
3. `standard.standardUpstream.remoteOptions()` (`standard.go:39-58`) builds `remote.Option` slice with appropriate `authn` config
4. All upstream operations (HEAD/GET manifest, HEAD/GET blob) use these options

## Client-Facing Authentication

**Not implemented.** All incoming requests to barnacle are unauthenticated.

### Current Behavior

| Endpoint | Expected (OCI Spec) | Actual |
|----------|---------------------|--------|
| `GET /v2/` | `401` + `WWW-Authenticate` header (if auth required) | `200 OK` (always) |
| `GET /v2/<name>/manifests/<ref>` | `401` if unauthorized | Always serves content |
| `GET /v2/<name>/blobs/<digest>` | `401` if unauthorized | Always serves content |
| Management API (`/api/v1/*`) | N/A (not part of OCI spec) | Always serves content |

### OCI Distribution Spec Requirements

Per the [OCI Distribution Spec auth section](https://distribution.github.io/distribution/spec/auth/), when a registry requires authentication:

1. Client hits `GET /v2/` → registry returns `401` with:
   ```
   WWW-Authenticate: Bearer realm="<token-endpoint>",service="<registry>",scope="<scope>"
   ```
2. Client requests a token from the `realm` URL with the specified `scope`
3. Client retries with `Authorization: Bearer <token>`

Barnacle does not implement any part of this flow. The `ErrUnauthorized` error type exists in `internal/tk/httptk/errors.go:157-161` but is never used on client-facing routes.

### What Works Today

- Barnacle deployed behind a network boundary (VPN, internal cluster network) where all callers are trusted
- Kubelet pulling images through barnacle on the same cluster network

### What Doesn't Work

- Exposing barnacle to untrusted clients
- Per-repository access control
- Audit logging of who pulled what
- Multi-tenant environments

## Key Files

| Component | Path |
|-----------|------|
| Upstream auth config | `pkg/configuration/upstream.go` |
| Upstream auth usage | `internal/registry/upstream/standard/standard.go` |
| Distribution routes (no auth) | `internal/routes/distributionroutes/registry/controller.go` |
| OCI error types (UNAUTHORIZED defined) | `internal/tk/httptk/errors.go` |
| Upstream factory | `internal/registry/factory.go` |