//nolint:testpackage // Internal tests need access to unexported error types
package memory

import (
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) cache.ManifestCache {
	t.Helper()
	c, err := NewManifestCache(&CacheOptions{
		TagLimit:              100,
		ManifestMemoryLimitMi: 10,
	})
	require.NoError(t, err)
	return c
}

func makeImageManifest(size int64) *data.ImageManifestResponse {
	return &data.ImageManifestResponse{
		Manifest:    &v1.Manifest{SchemaVersion: 2},
		RawManifest: make([]byte, size),
		Digest: v1.Hash{
			Algorithm: "sha256",
			Hex:       "abc123",
		},
		Size:      size,
		MediaType: types.OCIManifestSchema1,
	}
}

func makeIndexManifest(size int64) *data.IndexManifestResponse {
	return &data.IndexManifestResponse{
		Manifest:    &v1.IndexManifest{SchemaVersion: 2},
		RawManifest: make([]byte, size),
		Digest: v1.Hash{
			Algorithm: "sha256",
			Hex:       "def456",
		},
		Size:      size,
		MediaType: types.OCIImageIndex,
	}
}

func TestNewManifestCache(t *testing.T) {
	t.Run("creates cache with valid options", func(t *testing.T) {
		c, err := NewManifestCache(&CacheOptions{
			TagLimit:              100,
			ManifestMemoryLimitMi: 10,
		})
		require.NoError(t, err)
		assert.NotNil(t, c)
	})
}

func TestMemoryManifestCache_PutAndGetImage(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/nginx"
	digest := "sha256:abc123"
	manifest := makeImageManifest(1024)

	t.Run("stores and retrieves image manifest", func(t *testing.T) {
		err := c.PutImage(upstream, repo, digest, manifest)
		require.NoError(t, err)

		// Ristretto is eventually consistent, wait for item to be available
		time.Sleep(10 * time.Millisecond)

		retrieved, err := c.GetImage(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest.Digest, retrieved.Digest)
		assert.Equal(t, manifest.Size, retrieved.Size)
		assert.Equal(t, manifest.MediaType, retrieved.MediaType)
	})

	t.Run("returns ErrItemNotFound for non-existent digest", func(t *testing.T) {
		_, err := c.GetImage(upstream, repo, "sha256:nonexistent")
		assert.ErrorIs(t, err, ErrItemNotFound)
	})

	t.Run("returns ErrItemNotFound for non-existent repo", func(t *testing.T) {
		_, err := c.GetImage(upstream, "nonexistent/repo", digest)
		assert.ErrorIs(t, err, ErrItemNotFound)
	})

	t.Run("returns ErrItemNotFound for non-existent upstream", func(t *testing.T) {
		_, err := c.GetImage("nonexistent", repo, digest)
		assert.ErrorIs(t, err, ErrItemNotFound)
	})
}

func TestMemoryManifestCache_PutAndGetIndex(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/alpine"
	digest := "sha256:def456"
	manifest := makeIndexManifest(2048)

	t.Run("stores and retrieves index manifest", func(t *testing.T) {
		err := c.PutIndex(upstream, repo, digest, manifest)
		require.NoError(t, err)

		// Ristretto is eventually consistent
		time.Sleep(10 * time.Millisecond)

		retrieved, err := c.GetIndex(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest.Digest, retrieved.Digest)
		assert.Equal(t, manifest.Size, retrieved.Size)
		assert.Equal(t, manifest.MediaType, retrieved.MediaType)
	})

	t.Run("returns ErrItemNotFound for non-existent digest", func(t *testing.T) {
		_, err := c.GetIndex(upstream, repo, "sha256:nonexistent")
		assert.ErrorIs(t, err, ErrItemNotFound)
	})
}

func TestMemoryManifestCache_TypeMismatch(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/test"

	t.Run("GetImage returns cache.ErrItemTypeMismatch for index manifest", func(t *testing.T) {
		digest := "sha256:index123"
		indexManifest := makeIndexManifest(1024)

		err := c.PutIndex(upstream, repo, digest, indexManifest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		_, err = c.GetImage(upstream, repo, digest)
		assert.ErrorIs(t, err, cache.ErrItemTypeMismatch)
	})

	t.Run("GetIndex returns cache.ErrItemTypeMismatch for image manifest", func(t *testing.T) {
		digest := "sha256:image123"
		imageManifest := makeImageManifest(1024)

		err := c.PutImage(upstream, repo, digest, imageManifest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		_, err = c.GetIndex(upstream, repo, digest)
		assert.ErrorIs(t, err, cache.ErrItemTypeMismatch)
	})
}

func TestMemoryManifestCache_GetType(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/test"

	t.Run("returns ManifestTypeImage for image manifest", func(t *testing.T) {
		digest := "sha256:img1"
		err := c.PutImage(upstream, repo, digest, makeImageManifest(512))
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		manifestType, err := c.GetType(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, cache.ManifestTypeImage, manifestType)
	})

	t.Run("returns ManifestTypeIndex for index manifest", func(t *testing.T) {
		digest := "sha256:idx1"
		err := c.PutIndex(upstream, repo, digest, makeIndexManifest(512))
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		manifestType, err := c.GetType(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, cache.ManifestTypeIndex, manifestType)
	})

	t.Run("returns ErrItemNotFound for non-existent item", func(t *testing.T) {
		_, err := c.GetType(upstream, repo, "sha256:nonexistent")
		assert.ErrorIs(t, err, ErrItemNotFound)
	})
}

func TestMemoryManifestCache_GetUntyped(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/test"

	t.Run("returns image manifest with correct type", func(t *testing.T) {
		digest := "sha256:untyped-img"
		imageManifest := makeImageManifest(256)
		err := c.PutImage(upstream, repo, digest, imageManifest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		elem, elemType, err := c.GetUntyped(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, cache.ManifestTypeImage, elemType)
		assert.NotNil(t, elem)

		// Verify we can cast to the correct type
		img, ok := elem.(*data.ImageManifestResponse)
		assert.True(t, ok)
		assert.Equal(t, imageManifest.Digest, img.Digest)
	})

	t.Run("returns index manifest with correct type", func(t *testing.T) {
		digest := "sha256:untyped-idx"
		indexManifest := makeIndexManifest(256)
		err := c.PutIndex(upstream, repo, digest, indexManifest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		elem, elemType, err := c.GetUntyped(upstream, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, cache.ManifestTypeIndex, elemType)
		assert.NotNil(t, elem)

		// Verify we can cast to the correct type
		idx, ok := elem.(*data.IndexManifestResponse)
		assert.True(t, ok)
		assert.Equal(t, indexManifest.Digest, idx.Digest)
	})

	t.Run("returns ErrItemNotFound for non-existent item", func(t *testing.T) {
		_, _, err := c.GetUntyped(upstream, repo, "sha256:nonexistent")
		assert.ErrorIs(t, err, ErrItemNotFound)
	})
}

func TestMemoryManifestCache_TagToDigest(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	repo := "library/nginx"
	tag := "latest"
	digest := "sha256:abc123def456"

	t.Run("stores and retrieves tag mapping", func(t *testing.T) {
		err := c.PutTag(upstream, repo, tag, digest)
		require.NoError(t, err)

		// Ristretto is eventually consistent
		time.Sleep(10 * time.Millisecond)

		retrieved, stale, err := c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, digest, retrieved)
		assert.False(t, stale, "tag should not be stale immediately after caching")
	})

	t.Run("returns ErrTagNotFound for non-existent tag", func(t *testing.T) {
		_, _, err := c.TagToDigest(upstream, repo, "nonexistent")
		assert.ErrorIs(t, err, ErrTagNotFound)
	})

	t.Run("returns ErrTagNotFound for non-existent repo", func(t *testing.T) {
		_, _, err := c.TagToDigest(upstream, "nonexistent/repo", tag)
		assert.ErrorIs(t, err, ErrTagNotFound)
	})

	t.Run("returns ErrTagNotFound for non-existent upstream", func(t *testing.T) {
		_, _, err := c.TagToDigest("nonexistent", repo, tag)
		assert.ErrorIs(t, err, ErrTagNotFound)
	})

	t.Run("can update tag to point to new digest", func(t *testing.T) {
		newDigest := "sha256:newdigest789"
		err := c.PutTag(upstream, repo, tag, newDigest)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		retrieved, stale, err := c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, newDigest, retrieved)
		assert.False(t, stale)
	})
}

func TestMemoryManifestCache_SameDigestDifferentRepos(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	digest := "sha256:shared123"
	repo1 := "library/nginx"
	repo2 := "library/alpine"

	manifest1 := makeImageManifest(1024)
	manifest2 := makeImageManifest(2048)

	t.Run("same digest in different repos stored separately", func(t *testing.T) {
		err := c.PutImage(upstream, repo1, digest, manifest1)
		require.NoError(t, err)

		err = c.PutImage(upstream, repo2, digest, manifest2)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		retrieved1, err := c.GetImage(upstream, repo1, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest1.Size, retrieved1.Size)

		retrieved2, err := c.GetImage(upstream, repo2, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest2.Size, retrieved2.Size)
	})
}

func TestMemoryManifestCache_SameTagDifferentRepos(t *testing.T) {
	c := newTestCache(t)
	upstream := "dockerhub"
	tag := "latest"
	repo1 := "library/nginx"
	digest1 := "sha256:nginx123"

	// Test that updating the same tag in the same repo works correctly
	// This verifies key construction without fighting ristretto's admission policy
	t.Run("tag updates work correctly", func(t *testing.T) {
		putErr := c.PutTag(upstream, repo1, tag, digest1)
		require.NoError(t, putErr)

		// Wait for value to be available
		require.Eventually(t, func() bool {
			r, _, getErr := c.TagToDigest(upstream, repo1, tag)
			return getErr == nil && r == digest1
		}, 500*time.Millisecond, time.Millisecond, "tag should be available")

		// Now update the same tag
		newDigest := "sha256:updated789"
		putErr = c.PutTag(upstream, repo1, tag, newDigest)
		require.NoError(t, putErr)

		// Wait for update to be available
		require.Eventually(t, func() bool {
			r, _, getErr := c.TagToDigest(upstream, repo1, tag)
			return getErr == nil && r == newDigest
		}, 500*time.Millisecond, time.Millisecond, "updated tag should be available")

		retrieved, _, getErr := c.TagToDigest(upstream, repo1, tag)
		require.NoError(t, getErr)
		assert.Equal(t, newDigest, retrieved)
	})
}

func TestMemoryManifestCache_SameDigestDifferentUpstreams(t *testing.T) {
	c := newTestCache(t)
	digest := "sha256:shared123"
	repo := "library/nginx"
	upstream1 := "dockerhub"
	upstream2 := "ghcr"

	manifest1 := makeImageManifest(1024)
	manifest2 := makeImageManifest(2048)

	t.Run("same digest in different upstreams stored separately", func(t *testing.T) {
		err := c.PutImage(upstream1, repo, digest, manifest1)
		require.NoError(t, err)

		err = c.PutImage(upstream2, repo, digest, manifest2)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		retrieved1, err := c.GetImage(upstream1, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest1.Size, retrieved1.Size)

		retrieved2, err := c.GetImage(upstream2, repo, digest)
		require.NoError(t, err)
		assert.Equal(t, manifest2.Size, retrieved2.Size)
	})
}

func TestMemoryManifestCache_TagTTL(t *testing.T) {
	upstream := "dockerhub"
	repo := "library/nginx"
	tag := "latest"
	digest := "sha256:abc123"

	t.Run("tag becomes stale after TTL expires", func(t *testing.T) {
		currentTime := time.Now()
		c, err := NewManifestCache(&CacheOptions{
			TagLimit:              100,
			ManifestMemoryLimitMi: 10,
			TagTTL:                5 * time.Minute,
		})
		require.NoError(t, err)

		// Override the now function for testing
		mc := c.(*memoryManifestCache)
		mc.now = func() time.Time { return currentTime }

		// Store the tag
		err = c.PutTag(upstream, repo, tag, digest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Immediately after caching, tag should not be stale
		retrieved, stale, err := c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, digest, retrieved)
		assert.False(t, stale, "tag should not be stale immediately")

		// Advance time by 4 minutes (still within TTL)
		mc.now = func() time.Time { return currentTime.Add(4 * time.Minute) }

		retrieved, stale, err = c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, digest, retrieved)
		assert.False(t, stale, "tag should not be stale before TTL")

		// Advance time by 6 minutes (past TTL)
		mc.now = func() time.Time { return currentTime.Add(6 * time.Minute) }

		retrieved, stale, err = c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, digest, retrieved)
		assert.True(t, stale, "tag should be stale after TTL")
	})

	t.Run("zero TTL means tags never become stale", func(t *testing.T) {
		currentTime := time.Now()
		c, err := NewManifestCache(&CacheOptions{
			TagLimit:              100,
			ManifestMemoryLimitMi: 10,
			TagTTL:                0, // Zero TTL
		})
		require.NoError(t, err)

		// Override the now function for testing
		mc := c.(*memoryManifestCache)
		mc.now = func() time.Time { return currentTime }

		// Store the tag
		err = c.PutTag(upstream, repo, tag, digest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Even after a very long time, tag should not be stale
		mc.now = func() time.Time { return currentTime.Add(24 * time.Hour) }

		retrieved, stale, err := c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, digest, retrieved)
		assert.False(t, stale, "tag should never be stale with zero TTL")
	})

	t.Run("updating tag resets staleness timer", func(t *testing.T) {
		currentTime := time.Now()
		c, err := NewManifestCache(&CacheOptions{
			TagLimit:              100,
			ManifestMemoryLimitMi: 10,
			TagTTL:                5 * time.Minute,
		})
		require.NoError(t, err)

		// Override the now function for testing
		mc := c.(*memoryManifestCache)
		mc.now = func() time.Time { return currentTime }

		// Store the tag
		err = c.PutTag(upstream, repo, tag, digest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Advance time by 4 minutes
		mc.now = func() time.Time { return currentTime.Add(4 * time.Minute) }

		// Update the tag with the same digest
		newDigest := "sha256:updated456"
		err = c.PutTag(upstream, repo, tag, newDigest)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Now at original_time + 4 minutes, tag should not be stale
		// (because we just updated it)
		retrieved, stale, err := c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, newDigest, retrieved)
		assert.False(t, stale, "tag should not be stale right after update")

		// Advance time by another 4 minutes (8 minutes from original, 4 from update)
		mc.now = func() time.Time { return currentTime.Add(8 * time.Minute) }

		retrieved, stale, err = c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, newDigest, retrieved)
		assert.False(t, stale, "tag should still be fresh 4 minutes after update")

		// Advance time by another 2 minutes (6 minutes from update = past TTL)
		mc.now = func() time.Time { return currentTime.Add(10 * time.Minute) }

		retrieved, stale, err = c.TagToDigest(upstream, repo, tag)
		require.NoError(t, err)
		assert.Equal(t, newDigest, retrieved)
		assert.True(t, stale, "tag should be stale 6 minutes after update")
	})
}
