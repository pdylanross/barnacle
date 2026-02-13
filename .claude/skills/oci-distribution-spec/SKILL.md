---
name: oci-distribution-spec
description: Reference for the OCI Distribution Spec (Docker Registry HTTP API V2). Use when working with container registries, implementing registry clients/proxies, handling image pulls/pushes, blob uploads, manifest operations, or debugging registry API interactions. Covers endpoints, digests, authentication, error codes, and resumable uploads.
---

# OCI Distribution Spec Reference

The OCI Distribution Spec (Docker Registry HTTP API V2) defines the protocol for distributing container images. This skill provides quick reference for implementing registry clients, proxies, or debugging registry interactions.

## Core Concepts

### Content Addressing
All content is addressed by **digest**: `algorithm:hex` (e.g., `sha256:abc123...`). Digests enable verification - fetch by digest, verify content matches.

### URL Structure
```
/v2/<repository>/...
```
Repository names: lowercase alphanumeric, may include `.`, `-`, `_`, separated by `/`. Max 256 chars total.

### Key Headers
| Header | Purpose |
|--------|---------|
| `Docker-Content-Digest` | Digest of returned content |
| `Docker-Distribution-API-Version` | `registry/2.0` |
| `Docker-Upload-UUID` | Upload session identifier |
| `WWW-Authenticate` | Auth challenge on 401 |
| `Link` | Pagination (RFC5988 format) |

## Quick Reference

### Version Check
```http
GET /v2/
```
Returns `200 OK` if V2 API supported, `401` if auth required, `404` if V2 not implemented.

### Manifests

**Get manifest:**
```http
GET /v2/<name>/manifests/<reference>
Accept: application/vnd.docker.distribution.manifest.v2+json
```
`reference` = tag or digest. Response includes `Docker-Content-Digest` header.

**Check existence:**
```http
HEAD /v2/<name>/manifests/<reference>
```

**Push manifest:**
```http
PUT /v2/<name>/manifests/<reference>
Content-Type: <manifest-media-type>
```
Returns `201 Created` with `Location` and `Docker-Content-Digest`.

**Delete manifest:**
```http
DELETE /v2/<name>/manifests/<digest>
```
Must use digest, not tag. Returns `202 Accepted`.

### Blobs (Layers)

**Get blob:**
```http
GET /v2/<name>/blobs/<digest>
```
May return `307` redirect. Supports `Range` header for partial downloads.

**Check existence:**
```http
HEAD /v2/<name>/blobs/<digest>
```
Returns `200` with `Content-Length` and `Docker-Content-Digest` if exists.

**Delete blob:**
```http
DELETE /v2/<name>/blobs/<digest>
```

### Blob Uploads

**Monolithic upload (single request):**
```http
POST /v2/<name>/blobs/uploads/?digest=<digest>
Content-Type: application/octet-stream

<binary data>
```

**Resumable upload flow:**
```http
# 1. Initiate
POST /v2/<name>/blobs/uploads/
# Returns 202 with Location: /v2/<name>/blobs/uploads/<uuid>

# 2. Upload chunks
PATCH /v2/<name>/blobs/uploads/<uuid>
Content-Range: <start>-<end>
Content-Type: application/octet-stream

# 3. Complete
PUT /v2/<name>/blobs/uploads/<uuid>?digest=<digest>
```

**Cross-repository mount:**
```http
POST /v2/<name>/blobs/uploads/?mount=<digest>&from=<source-repo>
```
Returns `201` if mounted, `202` if falls back to upload.

**Check upload progress:**
```http
GET /v2/<name>/blobs/uploads/<uuid>
```
`Range` header shows progress.

**Cancel upload:**
```http
DELETE /v2/<name>/blobs/uploads/<uuid>
```

### Catalog & Tags

**List repositories:**
```http
GET /v2/_catalog?n=<count>&last=<marker>
```

**List tags:**
```http
GET /v2/<name>/tags/list?n=<count>&last=<marker>
```

Pagination: follow `Link` header with `rel="next"`.

## Common Error Codes

| Code | Meaning |
|------|---------|
| `BLOB_UNKNOWN` | Blob not found in repository |
| `BLOB_UPLOAD_INVALID` | Upload error, must restart |
| `BLOB_UPLOAD_UNKNOWN` | Upload session not found |
| `DIGEST_INVALID` | Content doesn't match digest |
| `MANIFEST_INVALID` | Manifest validation failed |
| `MANIFEST_UNKNOWN` | Manifest not found |
| `NAME_UNKNOWN` | Repository not found |
| `UNAUTHORIZED` | Authentication required |
| `DENIED` | Access denied |
| `UNSUPPORTED` | Operation not supported |
| `TOOMANYREQUESTS` | Rate limited |

Error response format:
```json
{
  "errors": [{
    "code": "ERROR_CODE",
    "message": "human readable",
    "detail": { ... }
  }]
}
```

## Implementation Notes

1. **Always verify digests** - After fetching by digest, compute digest of received content
2. **Handle redirects** - Blob GETs may return 307 to external storage
3. **Use chunked uploads** for large blobs - More resilient to network issues
4. **Check blob existence before upload** - Use HEAD to avoid duplicate uploads
5. **Mount blobs when possible** - Avoids re-uploading known content across repos
6. **Treat upload URLs as opaque** - Don't construct them, use Location header values
7. **Handle 5xx errors** - 502/503/504 are temporary, retry with backoff

## Detailed References

For comprehensive API endpoint details including all parameters, headers, and response codes:
- See [references/api-endpoints.md](references/api-endpoints.md)

For complete error code documentation:
- See [references/error-codes.md](references/error-codes.md)

## Official Documentation

Full specification: https://distribution.github.io/distribution/spec/api/

Related specs:
- Manifest format: https://distribution.github.io/distribution/spec/manifest-v2-2/
- Authentication: https://distribution.github.io/distribution/spec/auth/