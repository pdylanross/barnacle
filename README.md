# Barnacle

A distributed, tiered OCI registry caching proxy designed for terabyte-scale container images.

## Overview

Barnacle is a Kubernetes-native caching proxy for OCI registries that solves the challenges of managing extremely large container images (multi-terabyte blobs) across distributed infrastructure. Like its namesake that attaches to larger vessels, Barnacle attaches to upstream registries and intelligently caches their content across a distributed fleet of storage pods.

### Why Barnacle?

Modern container workloads increasingly include massive images containing ML models, scientific datasets, and other large artifacts. Traditional registry caching solutions face several challenges:

- **Fixed placement**: Consistent hashing locks blobs to specific nodes, preventing optimization
- **Inefficient storage**: All blobs treated equally regardless of access patterns, wasting expensive fast storage
- **Poor scalability**: Large blobs overwhelm memory-based caching systems
- **Limited capacity**: Cannot efficiently handle terabyte-scale individual blobs

Barnacle addresses these challenges through intelligent metadata-driven placement, automatic storage tiering, and streaming-first architecture.

## Key Features

- **Terabyte-Scale Blob Support**: Stream-based architecture handles blobs of any size without memory constraints
- **Metadata-Driven Sharding**: Intelligent placement based on capacity, load, and access patterns rather than static hashing
- **Automatic Storage Tiering**: Cost optimization through hot/warm/cold/archive tiers based on access patterns
- **Horizontal Scalability**: Add capacity by adding pods; no complex resharding required
- **Multiple Upstream Registries**: Proxy to Docker Hub, GCR, ECR, Harbor, and private registries simultaneously
- **Flexible Authentication**: Support for both credential passthrough and hardcoded upstream credentials
- **Thundering Herd Prevention**: Distributed locking ensures single upstream fetch per blob across the cluster
- **Read-Only Safe**: Production-ready read-only mode with architecture supporting write operations

## Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────┐
│                    Client Requests                       │
│              (docker pull, containerd)                   │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
         ┌───────────────────────┐
         │  Load Balancer/       │
         │  Ingress              │
         └───────────┬───────────┘
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
┌───────────────┐         ┌───────────────┐
│ Cache Pod 0   │  ◄────► │ Cache Pod N   │
│ (Router)      │         │ (Router)      │
└───────┬───────┘         └───────┬───────┘
        │                         │
        ▼                         ▼
┌───────────────┐         ┌───────────────┐
│ Storage Tier  │         │ Storage Tier  │
│ (NVMe/SSD/    │         │ (NVMe/SSD/    │
│  HDD/S3)      │         │  HDD/S3)      │
└───────────────┘         └───────────────┘
        │                         │
        └────────────┬────────────┘
                     ▼
              ┌─────────────┐
              │    Redis    │
              │  (Metadata) │
              └─────────────┘
```

### Core Components

#### 1. Router Component
Each cache pod runs a router that:
- Determines blob ownership via metadata lookup (not hashing)
- Issues 307 redirects to the appropriate storage pod
- Handles cache misses by fetching from upstream
- Manages distributed locks during fetch operations

#### 2. Redis Metadata Store
Redis serves as the source of truth for:
- **Blob locations**: Maps digests to storage pods and tiers
- **Node capacity**: Tracks available space, IOPS, and load per pod
- **Distributed locks**: Prevents duplicate fetches (thundering herd protection)
- **Access patterns**: Records access frequency for tier migration decisions
- **Membership**: Tracks active pods for placement decisions

**Why Redis over other options?**
- Fast metadata lookups (critical path for all requests)
- Built-in distributed locking with TTL (via Redlock/Redsync)
- Sorted sets for efficient access pattern tracking
- Pub/Sub for membership changes
- AOF persistence provides durability without Raft overhead
- Simple operations compared to etcd or Consul
- Single dependency for both metadata and coordination

#### 3. Metadata-Based Sharding

**Why metadata-based instead of consistent hashing?**

Consistent hashing (`digest → hash → pod`) is simple but inflexible:
- Cannot move blobs after initial placement
- No consideration of pod capacity or load
- Wastes space (some pods fill while others sit empty)
- Makes storage tiering impossible

Metadata-based sharding (`digest → Redis lookup → pod(s)`) provides:
- Dynamic placement based on current capacity
- Load-aware distribution
- Support for storage tiering (move blobs between storage classes)
- Graceful rebalancing without rehashing
- Multi-replica support with configurable redundancy

**Placement Algorithm:**
```
For new blob:
1. Query all pod capacities from Redis
2. Filter pods by available space
3. Score by: available_space (50%) + load (30%) + blob_count (20%)
4. Select N pods with best scores (N = replication factor)
5. Store metadata mapping digest → [pod1, pod2, ...]
```

#### 4. Storage Tiering

Blobs automatically migrate between storage tiers based on access patterns:

| Tier    | Storage     | Cost/GB | Use Case                    | Demotion After |
|---------|-------------|---------|----------------------------|----------------|
| Hot     | NVMe SSD    | $0.50   | Recently accessed (10+ hits) | 24 hours       |
| Warm    | SSD         | $0.15   | Moderate access (3+ hits)    | 7 days         |
| Cold    | HDD         | $0.05   | Occasional access            | 30 days        |
| Archive | S3 Glacier  | $0.004  | Rarely accessed              | 90+ days       |

**Background workers continuously:**
- Monitor access patterns
- Promote frequently-accessed blobs to faster tiers
- Demote cold blobs to cheaper storage
- Optimize cost vs. performance automatically

**Example cost savings for 100TB:**
- All NVMe: $50,000/month
- Tiered (10% hot, 20% warm, 40% cold, 30% archive): **$10,120/month** (80% reduction)

#### 5. Storage Backend

**Filesystem-based, not key-value stores**

**Why filesystem instead of BadgerDB/Pebble/etc?**
- No size limits (TB+ blobs)
- Native streaming via `io.Reader`/`io.Writer`
- OS page cache provides automatic hot-data caching
- Standard tools work (`du`, `find`, `rsync`)
- Simple debugging and operations
- Perfect range request support
- Works identically across all storage tiers

Blobs stored content-addressed:
```
/var/cache/oci/
  sha256/
    ab/
      cd/
        ef/
          abcdef123.../
            data       # actual blob content
            metadata   # size, timestamps, tier
```

Each storage tier (hot/warm/cold) is a separate filesystem mount with different backing storage.

### Request Flow

#### Cache Hit (Blob Exists)
```
1. Client → Load Balancer → Any Cache Pod
2. Pod queries Redis: "Where is sha256:abc...?"
3. Redis returns: [pod-2 (hot tier), pod-5 (warm tier)]
4. Pod issues 307 redirect to pod-2
5. Client fetches directly from pod-2
6. Pod-2 streams from local NVMe
7. Access recorded in Redis (async)
```

#### Cache Miss (Fetch from Upstream)
```
1. Client → Cache Pod → Redis lookup → Not found
2. Pod acquires distributed lock in Redis for digest
3. Lock acquired → fetch from upstream begins
4. Redis metadata queried for optimal placement
5. Selected pods: [pod-7 (hot), pod-3 (hot)] based on capacity
6. Blob streams from upstream → client (via pod)
7. Simultaneously streams to pod-7 and pod-3 storage
8. Metadata written to Redis on completion
9. Lock released
10. Future requests redirect to pod-7 or pod-3
```

#### Thundering Herd Prevention
```
1. Pod A: Acquires lock for sha256:abc... → SUCCESS
2. Pod B: Attempts lock for sha256:abc... → FAIL
3. Pod B: Returns 503 Retry-After: 5
4. Client waits 5 seconds, retries
5. Blob now cached, Pod B redirects to owner
```

The lock uses heartbeat extension: acquired with 30s TTL, extended every 10s while download progresses. If the downloading pod crashes, lock expires automatically.

### Multiple Upstream Registries

Barnacle proxies to any number of upstream registries simultaneously:

**Supported upstreams:**
- Docker Hub (`registry-1.docker.io`)
- Google Container Registry (`gcr.io`, `us.gcr.io`, etc.)
- AWS Elastic Container Registry (ECR)
- Azure Container Registry (ACR)
- GitHub Container Registry (`ghcr.io`)
- Harbor
- Quay.io
- Any OCI-compliant registry

**Authentication modes:**

1. **Hardcoded Credentials** (registry → proxy)
   ```yaml
   upstreams:
     - registry: "registry-1.docker.io"
       username: "company-bot"
       password: "secret123"
     - registry: "gcr.io"
       service_account_key: "/path/to/key.json"
   ```
   - Centralizes credential management
   - Users don't need upstream credentials
   - Simplifies compliance and auditing
   - Recommended for production

2. **Credential Passthrough** (client → upstream via proxy)
   ```yaml
   upstreams:
     - registry: "registry-1.docker.io"
       passthrough_auth: true
   ```
   - Client provides credentials in pull request
   - Proxy forwards to upstream
   - Useful for user-specific private images
   - Supports per-user quotas and permissions

Both modes can be used simultaneously for different registries.

### Distributed Locking

Redis-based distributed locking prevents multiple pods from fetching the same blob:

**Lock characteristics:**
- 30-second TTL with 10-second heartbeat extension
- Progress tracking: only extends if bytes are flowing
- Automatic expiration if holder crashes
- Waiters poll and retry with exponential backoff

**Lock lifecycle:**
```go
1. Acquire lock with 30s TTL
2. Start download
3. Heartbeat goroutine extends every 10s
4. Track progress (bytes read)
5. Only extend if making progress
6. On completion or error: release lock
7. If crash: lock expires in 30s
```

Stalled downloads (network issue, upstream timeout) automatically release locks when progress stops, allowing other pods to retry.

### Scalability

**Horizontal scaling:**
- Add pods → increase capacity and throughput
- New pods register in Redis membership
- Placement algorithm automatically uses new capacity
- No rebalancing required (optional background optimization)

**Storage capacity scaling:**
```
3 pods × 5TB PVCs = 15TB total
Replication factor = 2
Effective capacity = 7.5TB

Add 2 pods (5 total):
5 pods × 5TB = 25TB total
Effective capacity = 12.5TB
```

**Performance characteristics:**
- Most requests: Single redirect (307) → direct to owner
- Cache miss: Single fetch → distributed write → cached
- Lock contention: Only during cache misses for same digest
- Metadata lookups: Redis (sub-millisecond)

## Roadmap

### Current Status: v0.0.0 (Alpha)

📋 **Planned:**

**v0.1.0 (Beta):**
- Read-only proxy mode
- Metadata-based sharding
- Redis metadata store
- Distributed locking
- Multiple upstream support
- Hardcoded credentials
- Storage tiering architecture
- Background tier migration

**v0.2.0**
- Credential passthrough
- Comprehensive metrics
- Helm chart

**v0.3.0**
- Write support (push to cache)
- Garbage collection
- Admin API
- Grafana dashboards

**v0.4.0:**
- Multi-cluster federation
- Cross-region replication
- Advanced placement policies (cost optimization)
- Webhook for external placement decisions

**v1.0.0:**
- Production hardening
- Performance optimization
- Comprehensive documentation
- Reference architectures

## Design Philosophy

### Why These Choices?

**Metadata-driven over consistent hashing:**
- Flexibility > Simplicity
- Enables tiering, rebalancing, and capacity-aware placement
- Slight complexity increase worth the operational benefits

**Redis over etcd/Consul:**
- Performance: Faster for hot-path lookups
- Simplicity: Single dependency for metadata + locking
- Familiarity: Most teams already run Redis

**Filesystem over key-value stores:**
- No size limits for TB-scale blobs
- Streaming without memory pressure
- Operational simplicity (standard tools work)

**Tiering over uniform storage:**
- 80% cost savings in practice
- Performance where it matters (hot tier)
- Automatic optimization reduces ops burden

**Read-only first:**
- Safer for production introduction
- Most use cases are pull-heavy
- Architecture supports writes when ready

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Areas we'd love help:**
- Performance testing at scale
- Additional upstream registry integrations
- Metrics and dashboards
- Documentation improvements
- Bug reports and feature requests

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Acknowledgments

Inspired by:
- [Docker Distribution](https://github.com/distribution/distribution) - Registry implementation
- [Groupcache](https://github.com/golang/groupcache) - Distributed caching concepts
- [Harbor](https://goharbor.io/) - Enterprise registry features
- The barnacle - Nature's original attachment-based caching system

---

**Project Status:** Alpha - Not recommended for production use yet. APIs may change.

**Questions?** Open an issue or join our [Discord](https://discord.gg/barnacle).
