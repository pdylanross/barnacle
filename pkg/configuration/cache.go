package configuration

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Default disk cache settings.
const (
	DefaultDiskTier0Path       = "/var/cache/barnacle/blobs"
	DefaultDiskDescriptorLimit = 10000
	DefaultTierReservePercent  = 0.0 // No reservation by default
)

// CacheConfiguration contains settings for the caching layer.
type CacheConfiguration struct {
	// Memory contains in-memory cache settings.
	Memory MemoryCacheConfiguration `koanf:"memory"`

	// Disk contains disk-based cache settings.
	Disk DiskCacheConfiguration `koanf:"disk"`
}

// Validate checks that the cache configuration is valid.
func (c *CacheConfiguration) Validate() error {
	if err := c.Memory.Validate(); err != nil {
		return err
	}
	return c.Disk.Validate()
}

// DiskTierConfiguration contains settings for a single disk cache tier.
type DiskTierConfiguration struct {
	// Tier is the tier number. Lower numbers indicate higher priority tiers.
	// Tier 0 is the insertion tier where new blobs are initially stored.
	Tier int `koanf:"tier"`

	// Path is the root directory for blob storage for this tier.
	Path string `koanf:"path"`

	// ReservePercent is the fraction of disk space (0.0-1.0) to reserve for new blobs
	// during rebalancing. This prevents the rebalancer from filling the tier completely,
	// leaving headroom for new hot blobs. Defaults to 0.0 (no reservation).
	ReservePercent float64 `koanf:"reservePercent"`

	// SizeLimit is an optional maximum size for this tier's cache storage.
	// Accepts Kubernetes-style quantities: "5Gi", "10G", "500Mi", "1Ti", etc.
	// When set, capacity planning uses this value instead of the actual
	// filesystem size. If empty, the actual filesystem capacity is used.
	SizeLimit string `koanf:"sizeLimit"`
}

// GetReservePercent returns the reserve percent, using the default if not set.
func (t *DiskTierConfiguration) GetReservePercent() float64 {
	if t.ReservePercent == 0 {
		return DefaultTierReservePercent
	}
	return t.ReservePercent
}

// GetSizeLimitBytes parses SizeLimit and returns the value in bytes.
// Returns 0 if SizeLimit is empty (meaning no limit).
// Returns an error if the format is invalid.
func (t *DiskTierConfiguration) GetSizeLimitBytes() (uint64, error) {
	if t.SizeLimit == "" {
		return 0, nil
	}
	q, err := resource.ParseQuantity(t.SizeLimit)
	if err != nil {
		return 0, fmt.Errorf("invalid sizeLimit %q: %w", t.SizeLimit, err)
	}
	v := q.Value()
	if v < 0 {
		return 0, fmt.Errorf("sizeLimit %q must not be negative", t.SizeLimit)
	}
	return uint64(v), nil
}

// Validate checks that the disk tier configuration is valid.
func (t *DiskTierConfiguration) Validate() error {
	if t.Tier < 0 {
		return fmt.Errorf("%w: disk tier must be non-negative, got %d",
			ErrInvalidConfiguration, t.Tier)
	}
	if t.Path == "" {
		return fmt.Errorf("%w: disk tier %d path cannot be empty",
			ErrInvalidConfiguration, t.Tier)
	}
	if t.ReservePercent < 0 || t.ReservePercent >= 1.0 {
		return fmt.Errorf("%w: disk tier %d reservePercent must be >= 0 and < 1.0, got %f",
			ErrInvalidConfiguration, t.Tier, t.ReservePercent)
	}
	if t.SizeLimit != "" {
		if _, err := t.GetSizeLimitBytes(); err != nil {
			return fmt.Errorf("%w: disk tier %d: %w",
				ErrInvalidConfiguration, t.Tier, err)
		}
	}
	return nil
}

// DiskCacheConfiguration contains settings for the disk-based blob cache.
type DiskCacheConfiguration struct {
	// Tiers contains the disk cache tier configurations.
	// Tiers should be ordered by priority (tier 0 first).
	// At least one tier must be configured.
	Tiers []DiskTierConfiguration `koanf:"tiers"`

	// DescriptorLimit is the maximum number of blob descriptors to cache in memory.
	// This is used for fast lookups of blob metadata without reading from disk.
	DescriptorLimit int `koanf:"descriptorLimit"`
}

// Validate checks that the disk cache configuration is valid.
func (d *DiskCacheConfiguration) Validate() error {
	if d.DescriptorLimit < 0 {
		return fmt.Errorf("%w: disk descriptorLimit must be non-negative, got %d",
			ErrInvalidConfiguration, d.DescriptorLimit)
	}

	if len(d.Tiers) == 0 {
		return fmt.Errorf("%w: at least one disk cache tier must be configured",
			ErrInvalidConfiguration)
	}

	for i, tier := range d.Tiers {
		if err := tier.Validate(); err != nil {
			return fmt.Errorf("tier %d: %w", i, err)
		}
	}

	return nil
}

// MemoryCacheConfiguration contains settings for the in-memory cache.
type MemoryCacheConfiguration struct {
	// TagLimit is the maximum number of tag-to-digest mappings to cache.
	TagLimit int `koanf:"tagLimit"`

	// ManifestMemoryLimitMi is the maximum memory in mebibytes (MiB) for manifest storage.
	ManifestMemoryLimitMi int `koanf:"manifestMemoryLimitMi"`

	// TagTTL is the duration after which a cached tag-to-digest mapping is considered stale
	// and should be revalidated against the upstream registry. This allows detecting when
	// a tag (like "latest") has been updated to point to a new digest.
	TagTTL time.Duration `koanf:"tagTTL"`
}

// Validate checks that the memory cache configuration is valid.
func (m *MemoryCacheConfiguration) Validate() error {
	if m.TagLimit < 0 {
		return fmt.Errorf("%w: cache tagLimit must be non-negative, got %d",
			ErrInvalidConfiguration, m.TagLimit)
	}
	if m.ManifestMemoryLimitMi < 0 {
		return fmt.Errorf("%w: cache manifestMemoryLimitMi must be non-negative, got %d",
			ErrInvalidConfiguration, m.ManifestMemoryLimitMi)
	}
	if m.TagTTL < 0 {
		return fmt.Errorf("%w: cache tagTTL must be non-negative, got %v",
			ErrInvalidConfiguration, m.TagTTL)
	}
	return nil
}
