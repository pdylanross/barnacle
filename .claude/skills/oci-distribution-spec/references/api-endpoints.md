# OCI Distribution API Endpoints Reference

Complete endpoint reference for the OCI Distribution Spec (Docker Registry HTTP API V2).

## Base Endpoint

### GET /v2/

Check API version support.

**Request:**
```http
GET /v2/
Host: <registry>
Authorization: <scheme> <token>
```

**Responses:**
| Status | Meaning |
|--------|---------|
| 200 | V2 API supported |
| 401 | Auth required (check WWW-Authenticate) |
| 404 | V2 not implemented |

**Headers returned:** `Docker-Distribution-API-Version: registry/2.0`

---

## Manifest Endpoints

### GET /v2/<name>/manifests/<reference>

Fetch manifest by tag or digest.

**Parameters:**
| Name | Location | Description |
|------|----------|-------------|
| name | path | Repository name |
| reference | path | Tag or digest |
| Accept | header | Desired manifest media type(s) |

**Success Response (200):**
```http
200 OK
Docker-Content-Digest: <digest>
Content-Type: <manifest-media-type>

{manifest body}
```

**Common Accept values:**
- `application/vnd.docker.distribution.manifest.v2+json`
- `application/vnd.docker.distribution.manifest.list.v2+json`
- `application/vnd.oci.image.manifest.v1+json`
- `application/vnd.oci.image.index.v1+json`

### HEAD /v2/<name>/manifests/<reference>

Check manifest existence without fetching body.

**Success Response (200):**
```http
200 OK
Content-Length: <length>
Docker-Content-Digest: <digest>
```

### PUT /v2/<name>/manifests/<reference>

Upload/update a manifest.

**Request:**
```http
PUT /v2/<name>/manifests/<reference>
Content-Type: <manifest-media-type>

{manifest body}
```

**Success Response (201):**
```http
201 Created
Location: /v2/<name>/manifests/<digest>
Docker-Content-Digest: <digest>
```

**Error: Missing layers (400):**
```json
{
  "errors": [{
    "code": "BLOB_UNKNOWN",
    "message": "blob unknown to registry",
    "detail": {"digest": "<missing-digest>"}
  }]
}
```

### DELETE /v2/<name>/manifests/<reference>

Delete manifest. **Reference MUST be a digest.**

**Success Response:** `202 Accepted`

---

## Blob Endpoints

### GET /v2/<name>/blobs/<digest>

Fetch blob content.

**Success Response (200):**
```http
200 OK
Content-Length: <length>
Docker-Content-Digest: <digest>
Content-Type: application/octet-stream

<binary data>
```

**Redirect Response (307):**
```http
307 Temporary Redirect
Location: <blob-location>
Docker-Content-Digest: <digest>
```

**Range Request:**
```http
GET /v2/<name>/blobs/<digest>
Range: bytes=<start>-<end>
```

**Partial Response (206):**
```http
206 Partial Content
Content-Length: <chunk-length>
Content-Range: bytes <start>-<end>/<total>
```

### HEAD /v2/<name>/blobs/<digest>

Check blob existence.

**Success Response (200):**
```http
200 OK
Content-Length: <length>
Docker-Content-Digest: <digest>
```

### DELETE /v2/<name>/blobs/<digest>

Delete blob from repository.

**Success Response:**
```http
202 Accepted
Docker-Content-Digest: <digest>
```

---

## Blob Upload Endpoints

### POST /v2/<name>/blobs/uploads/

Initiate blob upload.

#### Resumable Upload

```http
POST /v2/<name>/blobs/uploads/
Content-Length: 0
```

**Response (202):**
```http
202 Accepted
Location: /v2/<name>/blobs/uploads/<uuid>
Range: 0-0
Docker-Upload-UUID: <uuid>
```

#### Monolithic Upload

```http
POST /v2/<name>/blobs/uploads/?digest=<digest>
Content-Length: <length>
Content-Type: application/octet-stream

<binary data>
```

**Success (201):**
```http
201 Created
Location: /v2/<name>/blobs/<digest>
Docker-Content-Digest: <digest>
```

#### Cross-Repository Mount

```http
POST /v2/<name>/blobs/uploads/?mount=<digest>&from=<source-repo>
Content-Length: 0
```

**Mount Success (201):**
```http
201 Created
Location: /v2/<name>/blobs/<digest>
Docker-Content-Digest: <digest>
```

**Mount Failed, Fallback to Upload (202):**
```http
202 Accepted
Location: /v2/<name>/blobs/uploads/<uuid>
```

### GET /v2/<name>/blobs/uploads/<uuid>

Check upload progress.

**Response (204):**
```http
204 No Content
Location: /v2/<name>/blobs/uploads/<uuid>
Range: 0-<offset>
Docker-Upload-UUID: <uuid>
```

### PATCH /v2/<name>/blobs/uploads/<uuid>

Upload chunk.

**Request:**
```http
PATCH /v2/<name>/blobs/uploads/<uuid>
Content-Length: <chunk-size>
Content-Range: <start>-<end>
Content-Type: application/octet-stream

<chunk data>
```

**Success (202):**
```http
202 Accepted
Location: /v2/<name>/blobs/uploads/<uuid>
Range: 0-<new-offset>
Docker-Upload-UUID: <uuid>
```

**Range Error (416):**
```http
416 Requested Range Not Satisfiable
Location: /v2/<name>/blobs/uploads/<uuid>
Range: 0-<last-valid-offset>
```

### PUT /v2/<name>/blobs/uploads/<uuid>

Complete upload.

**With final chunk:**
```http
PUT /v2/<name>/blobs/uploads/<uuid>?digest=<digest>
Content-Length: <chunk-size>
Content-Range: <start>-<end>
Content-Type: application/octet-stream

<final chunk>
```

**Without data (all chunks already uploaded):**
```http
PUT /v2/<name>/blobs/uploads/<uuid>?digest=<digest>
Content-Length: 0
```

**Success (201):**
```http
201 Created
Location: /v2/<name>/blobs/<digest>
Docker-Content-Digest: <digest>
```

### DELETE /v2/<name>/blobs/uploads/<uuid>

Cancel upload.

**Success:** `204 No Content`

---

## Catalog & Tags

### GET /v2/_catalog

List repositories.

**Request:**
```http
GET /v2/_catalog?n=<count>&last=<marker>
```

**Response:**
```http
200 OK
Link: </v2/_catalog?n=<n>&last=<last>>; rel="next"

{
  "repositories": ["repo1", "repo2", ...]
}
```

### GET /v2/<name>/tags/list

List tags for repository.

**Request:**
```http
GET /v2/<name>/tags/list?n=<count>&last=<marker>
```

**Response:**
```http
200 OK
Link: </v2/<name>/tags/list?n=<n>&last=<last>>; rel="next"

{
  "name": "<name>",
  "tags": ["tag1", "tag2", ...]
}
```

---

## Pagination

Both catalog and tags endpoints support pagination:

| Parameter | Description |
|-----------|-------------|
| n | Max entries to return |
| last | Marker from previous response |

Follow the `Link` header `rel="next"` URL for next page. Absence of `Link` header means no more results.

---

## Authentication

On `401 Unauthorized`, check `WWW-Authenticate` header:

```http
WWW-Authenticate: Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:name:pull"
```

Obtain token from realm, include in subsequent requests:
```http
Authorization: Bearer <token>
```

See https://distribution.github.io/distribution/spec/auth/ for full auth spec.