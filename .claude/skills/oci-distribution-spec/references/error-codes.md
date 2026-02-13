# OCI Distribution API Error Codes

Complete reference for error codes returned by OCI Distribution Spec compliant registries.

## Error Response Format

All errors are returned as JSON:

```json
{
  "errors": [
    {
      "code": "<ERROR_CODE>",
      "message": "<human readable message>",
      "detail": <optional structured data>
    }
  ]
}
```

Multiple errors may be returned in a single response.

## Error Codes

### BLOB_UNKNOWN

**Message:** blob unknown to registry

**When returned:**
- GET/HEAD blob that doesn't exist
- Manifest references a blob not in the registry

**Detail:** May include `{"digest": "<digest>"}` identifying the unknown blob.

**Resolution:** Upload the missing blob before retrying.

---

### BLOB_UPLOAD_INVALID

**Message:** blob upload invalid

**When returned:**
- Upload session encountered an unrecoverable error
- Invalid upload state

**Resolution:** Cancel the upload and start a new upload session.

---

### BLOB_UPLOAD_UNKNOWN

**Message:** blob upload unknown to registry

**When returned:**
- GET/PATCH/PUT/DELETE on upload UUID that doesn't exist
- Upload was cancelled or timed out

**Resolution:** Start a new upload session.

---

### DIGEST_INVALID

**Message:** provided digest did not match uploaded content

**When returned:**
- Completing blob upload with wrong digest
- Manifest contains invalid layer digest

**Detail:** `{"digest": "<invalid-digest>"}`

**Resolution:** Verify digest calculation and retry with correct digest.

---

### MANIFEST_BLOB_UNKNOWN

**Message:** blob unknown to registry

**When returned:**
- Pushing manifest that references blobs not in registry

**Resolution:** Upload all referenced blobs before pushing manifest.

---

### MANIFEST_INVALID

**Message:** manifest invalid

**When returned:**
- Manifest fails validation
- Invalid JSON structure
- Missing required fields

**Detail:** Contains information about the validation failure.

**Resolution:** Fix manifest structure and retry.

---

### MANIFEST_UNKNOWN

**Message:** manifest unknown

**When returned:**
- GET/HEAD/DELETE manifest that doesn't exist
- Tag or digest not found

**Resolution:** Verify repository name and reference.

---

### MANIFEST_UNVERIFIED

**Message:** manifest failed signature verification

**When returned:**
- Manifest signature validation failed (legacy v1 manifests)

**Resolution:** Re-sign manifest with valid key.

---

### NAME_INVALID

**Message:** invalid repository name

**When returned:**
- Repository name doesn't match naming rules
- Invalid characters in name

**Valid name format:**
- Components: `[a-z0-9]+(?:[._-][a-z0-9]+)*`
- Separated by `/`
- Total length < 256 characters

**Resolution:** Fix repository name format.

---

### NAME_UNKNOWN

**Message:** repository name not known to registry

**When returned:**
- Operation on repository that doesn't exist
- No manifests/blobs pushed to repository yet

**Resolution:** Verify repository name, push content first.

---

### PAGINATION_NUMBER_INVALID

**Message:** invalid number of results requested

**When returned:**
- `n` parameter is not an integer
- `n` is negative
- `n` exceeds maximum allowed

**Resolution:** Use valid positive integer within limits.

---

### RANGE_INVALID

**Message:** invalid content range

**When returned:**
- Chunked upload with out-of-order range
- Content-Range doesn't match expected offset

**Resolution:** Check Range header from previous response, upload correct range.

---

### SIZE_INVALID

**Message:** provided length did not match content length

**When returned:**
- Content-Length header doesn't match body size

**Resolution:** Set correct Content-Length header.

---

### TAG_INVALID

**Message:** manifest tag did not match URI

**When returned:**
- Tag in manifest body doesn't match URI tag (legacy v1)

**Resolution:** Ensure tag consistency.

---

### UNAUTHORIZED

**Message:** authentication required

**When returned:**
- No credentials provided
- Invalid credentials
- Token expired

**Response includes:** `WWW-Authenticate` header with auth instructions.

**Resolution:** Obtain valid token and retry with `Authorization` header.

---

### DENIED

**Message:** requested access to the resource is denied

**When returned:**
- Valid auth but insufficient permissions
- Action not allowed for this user/scope

**Resolution:** Request appropriate permissions or use different credentials.

---

### UNSUPPORTED

**Message:** The operation is unsupported

**When returned:**
- Registry configured as pull-through cache (push disabled)
- Delete disabled in registry config
- Feature not implemented

**Resolution:** Check registry configuration or use different operation.

---

### TOOMANYREQUESTS

**Message:** too many requests

**When returned:**
- Rate limit exceeded

**Resolution:** Implement backoff and retry. Check `Retry-After` header if present.

---

## HTTP Status Code Summary

| Status | Common Causes |
|--------|--------------|
| 400 | Invalid request, bad name/digest/range |
| 401 | Authentication required |
| 403 | Permission denied |
| 404 | Resource not found |
| 405 | Method not allowed (e.g., delete disabled) |
| 416 | Range not satisfiable |
| 429 | Rate limited |
| 5xx | Server error, retry with backoff |

## Client Handling Best Practices

1. **Parse error codes, not messages** - Messages may change, codes are stable
2. **Handle unknown codes as UNKNOWN** - New codes may be added
3. **Check all errors in array** - Multiple errors may be present
4. **Use detail for debugging** - Contains useful diagnostic info
5. **Retry 5xx with backoff** - Server errors are often transient
6. **Don't retry 4xx** (except 429) - Client must fix the issue first