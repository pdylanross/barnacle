package configuration_test

import (
	"testing"

	"github.com/pdylanross/barnacle/pkg/configuration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskTierConfiguration_Validate(t *testing.T) {
	t.Run("valid tier", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier: 0,
			Path: "/var/cache/tier0",
		}
		err := tier.Validate()
		assert.NoError(t, err)
	})

	t.Run("negative tier", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier: -1,
			Path: "/var/cache/tier",
		}
		err := tier.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("empty path", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier: 0,
			Path: "",
		}
		err := tier.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("valid sizeLimit", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "5Gi",
		}
		err := tier.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid sizeLimit", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "invalid",
		}
		err := tier.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})
}

func TestDiskTierConfiguration_GetSizeLimitBytes(t *testing.T) {
	t.Run("empty sizeLimit returns 0", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier: 0,
			Path: "/var/cache/tier0",
		}
		limit, err := tier.GetSizeLimitBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), limit)
	})

	t.Run("gibibytes", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "5Gi",
		}
		limit, err := tier.GetSizeLimitBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(5*1024*1024*1024), limit)
	})

	t.Run("mebibytes", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "500Mi",
		}
		limit, err := tier.GetSizeLimitBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(500*1024*1024), limit)
	})

	t.Run("gigabytes decimal", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "10G",
		}
		limit, err := tier.GetSizeLimitBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(10*1000*1000*1000), limit)
	})

	t.Run("plain bytes", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "1073741824",
		}
		limit, err := tier.GetSizeLimitBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(1073741824), limit)
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		tier := configuration.DiskTierConfiguration{
			Tier:      0,
			Path:      "/var/cache/tier0",
			SizeLimit: "invalid",
		}
		_, err := tier.GetSizeLimitBytes()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid sizeLimit")
	})
}

func TestDiskCacheConfiguration_Validate(t *testing.T) {
	t.Run("valid with tiers", func(t *testing.T) {
		config := configuration.DiskCacheConfiguration{
			Tiers: []configuration.DiskTierConfiguration{
				{Tier: 0, Path: "/var/cache/tier0"},
				{Tier: 1, Path: "/var/cache/tier1"},
			},
			DescriptorLimit: 1000,
		}
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid - no tiers configured", func(t *testing.T) {
		config := configuration.DiskCacheConfiguration{
			DescriptorLimit: 1000,
		}
		err := config.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("negative descriptor limit", func(t *testing.T) {
		config := configuration.DiskCacheConfiguration{
			Tiers: []configuration.DiskTierConfiguration{
				{Tier: 0, Path: "/var/cache/blobs"},
			},
			DescriptorLimit: -1,
		}
		err := config.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("invalid tier in tiers array", func(t *testing.T) {
		config := configuration.DiskCacheConfiguration{
			Tiers: []configuration.DiskTierConfiguration{
				{Tier: 0, Path: "/var/cache/tier0"},
				{Tier: -1, Path: "/var/cache/invalid"},
			},
		}
		err := config.Validate()
		require.Error(t, err)
	})
}

func TestMemoryCacheConfiguration_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		config := configuration.MemoryCacheConfiguration{
			TagLimit:              1000,
			ManifestMemoryLimitMi: 100,
			TagTTL:                0,
		}
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("negative tag limit", func(t *testing.T) {
		config := configuration.MemoryCacheConfiguration{
			TagLimit: -1,
		}
		err := config.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("negative manifest memory limit", func(t *testing.T) {
		config := configuration.MemoryCacheConfiguration{
			ManifestMemoryLimitMi: -1,
		}
		err := config.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})

	t.Run("negative tag TTL", func(t *testing.T) {
		config := configuration.MemoryCacheConfiguration{
			TagTTL: -1,
		}
		err := config.Validate()
		require.Error(t, err)
		assert.ErrorIs(t, err, configuration.ErrInvalidConfiguration)
	})
}

func TestCacheConfiguration_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		config := configuration.CacheConfiguration{
			Memory: configuration.MemoryCacheConfiguration{
				TagLimit:              1000,
				ManifestMemoryLimitMi: 100,
			},
			Disk: configuration.DiskCacheConfiguration{
				Tiers: []configuration.DiskTierConfiguration{
					{Tier: 0, Path: "/var/cache/blobs"},
				},
				DescriptorLimit: 1000,
			},
		}
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid memory config", func(t *testing.T) {
		config := configuration.CacheConfiguration{
			Memory: configuration.MemoryCacheConfiguration{
				TagLimit: -1,
			},
			Disk: configuration.DiskCacheConfiguration{
				Tiers: []configuration.DiskTierConfiguration{
					{Tier: 0, Path: "/var/cache/blobs"},
				},
			},
		}
		err := config.Validate()
		require.Error(t, err)
	})

	t.Run("invalid disk config", func(t *testing.T) {
		config := configuration.CacheConfiguration{
			Memory: configuration.MemoryCacheConfiguration{},
			Disk: configuration.DiskCacheConfiguration{
				Tiers: []configuration.DiskTierConfiguration{
					{Tier: 0, Path: "/var/cache/blobs"},
				},
				DescriptorLimit: -1,
			},
		}
		err := config.Validate()
		require.Error(t, err)
	})
}
