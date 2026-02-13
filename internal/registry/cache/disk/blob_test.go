package disk_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/registry/cache/disk"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants for upstream and repo (ignored by disk cache but required by interface).
const (
	testUpstream = ""
	testRepo     = ""
)

// testDigest creates a valid sha256 digest string for testing.
// It pads short hex values to 64 characters using valid hex chars.
func testDigest(hex string) string {
	if len(hex) < 64 {
		hex += strings.Repeat("a", 64-len(hex))
	}
	return "sha256:" + hex[:64]
}

// testDescriptor creates a test descriptor with the given parameters.
// The digest must be a valid format (e.g., "sha256:" followed by 64 hex chars).
func testDescriptor(digest string, size int64, mediaType types.MediaType) *v1.Descriptor {
	h, err := v1.NewHash(digest)
	if err != nil {
		// For tests with intentionally invalid digests, return descriptor without hash
		return &v1.Descriptor{
			Size:      size,
			MediaType: mediaType,
		}
	}
	return &v1.Descriptor{
		Digest:    h,
		Size:      size,
		MediaType: mediaType,
	}
}

func newTestCache(t *testing.T) cache.BlobCache {
	t.Helper()
	tmpDir := t.TempDir()
	c, err := disk.NewBlobCache(&disk.BlobCacheOptions{
		BasePath: tmpDir,
	})
	require.NoError(t, err)
	return c
}

func TestNewBlobCache(t *testing.T) {
	t.Run("creates cache with valid options", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := disk.NewBlobCache(&disk.BlobCacheOptions{
			BasePath: tmpDir,
		})
		require.NoError(t, err)
		assert.NotNil(t, c)
	})

	t.Run("uses default path when not specified", func(t *testing.T) {
		c, err := disk.NewBlobCache(&disk.BlobCacheOptions{})
		require.NoError(t, err)
		assert.NotNil(t, c)
		// Clean up
		_ = os.RemoveAll(disk.DefaultBasePath)
	})

	t.Run("creates base directory if it doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		newPath := filepath.Join(tmpDir, "nested", "cache", "dir")
		c, err := disk.NewBlobCache(&disk.BlobCacheOptions{
			BasePath: newPath,
		})
		require.NoError(t, err)
		assert.NotNil(t, c)

		// Verify directory was created
		info, err := os.Stat(newPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("accepts custom descriptor limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := disk.NewBlobCache(&disk.BlobCacheOptions{
			BasePath:        tmpDir,
			DescriptorLimit: 5000,
		})
		require.NoError(t, err)
		assert.NotNil(t, c)
	})
}

func TestDiskBlobCache_PutAndGet(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("abc123def456789")
	content := []byte("test blob content")
	desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

	t.Run("stores and retrieves blob", func(t *testing.T) {
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		defer reader.Close()

		retrieved, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, content, retrieved)
	})

	t.Run("returns ErrBlobNotFound for non-existent digest", func(t *testing.T) {
		_, err := c.Get(context.Background(), testUpstream, testRepo, "sha256:nonexistent")
		assert.ErrorIs(t, err, cache.ErrBlobNotFound)
	})
}

func TestDiskBlobCache_Head(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("e1a2b3c4d5")
	content := []byte("blob data")
	desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

	t.Run("returns ErrBlobNotFound for non-existent blob", func(t *testing.T) {
		_, err := c.Head(context.Background(), testUpstream, testRepo, testDigest("doesnotexist"))
		assert.ErrorIs(t, err, cache.ErrBlobNotFound)
	})

	t.Run("returns descriptor for existing blob", func(t *testing.T) {
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		gotDesc, err := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		assert.Equal(t, desc.Size, gotDesc.Size)
		assert.Equal(t, desc.MediaType, gotDesc.MediaType)
		assert.Equal(t, desc.Digest.String(), gotDesc.Digest.String())
	})

	t.Run("returns descriptor from memory cache on second call", func(t *testing.T) {
		// First call loads from disk, second should be from memory
		gotDesc1, err := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		gotDesc2, err := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		assert.Equal(t, gotDesc1.Size, gotDesc2.Size)
		assert.Equal(t, gotDesc1.MediaType, gotDesc2.MediaType)
	})
}

func TestDiskBlobCache_DescriptorPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	digest := testDigest("bef0123456789abc")
	content := []byte("persistent content")
	desc := testDescriptor(digest, int64(len(content)), types.OCILayer)

	// Create cache and store blob
	c1, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	err = c1.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
	require.NoError(t, err)

	// Create new cache instance (simulating restart)
	c2, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	// Descriptor should be loadable from disk
	gotDesc, err := c2.Head(context.Background(), testUpstream, testRepo, digest)
	require.NoError(t, err)
	assert.Equal(t, desc.Size, gotDesc.Size)
	assert.Equal(t, desc.MediaType, gotDesc.MediaType)

	// Content should also be available
	reader, err := c2.Get(context.Background(), testUpstream, testRepo, digest)
	require.NoError(t, err)
	defer reader.Close()

	retrieved, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, retrieved)
}

func TestDiskBlobCache_ScansDiskOnInit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple blobs with first cache instance
	digests := []string{
		testDigest("1111111111111111"),
		testDigest("2222222222222222"),
		testDigest("3333333333333333"),
	}

	c1, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	for i, digest := range digests {
		content := []byte(strings.Repeat(string(rune('a'+i)), 100))
		desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)
		err = c1.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)
	}

	// Create new cache instance - should scan and find existing blobs
	c2, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	// All blobs should be accessible without needing to call Put again
	for i, digest := range digests {
		t.Run("blob "+digest[:20], func(t *testing.T) {
			// Get should work immediately
			reader, getErr := c2.Get(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, getErr)

			content, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			_ = reader.Close()

			expected := []byte(strings.Repeat(string(rune('a'+i)), 100))
			assert.Equal(t, expected, content)

			// Head should also work
			desc, headErr := c2.Head(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, headErr)
			assert.Equal(t, int64(len(expected)), desc.Size)
		})
	}
}

func TestDiskBlobCache_ScansDiskWithMultipleAlgorithms(t *testing.T) {
	tmpDir := t.TempDir()

	// Create blobs with different algorithm prefixes
	digests := []string{
		"sha256:" + strings.Repeat("a", 64),
		"sha384:" + strings.Repeat("b", 96),
		"sha512:" + strings.Repeat("c", 128),
	}

	c1, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	for _, digest := range digests {
		content := []byte("content for " + digest[:10])
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}
		err = c1.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)
	}

	// Create new cache instance
	c2, err := disk.NewBlobCache(&disk.BlobCacheOptions{BasePath: tmpDir})
	require.NoError(t, err)

	// All blobs from different algorithms should be found
	for _, digest := range digests {
		t.Run("algorithm "+digest[:6], func(t *testing.T) {
			reader, getErr := c2.Get(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, getErr)
			_ = reader.Close()
		})
	}
}

func TestDiskBlobCache_Delete(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("de1e7e123456")
	content := []byte("blob to delete")
	desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

	t.Run("deletes existing blob and descriptor", func(t *testing.T) {
		// First, put a blob
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		// Verify it exists
		_, err = c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		// Delete it
		err = c.Delete(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		// Verify it's gone
		_, err = c.Head(context.Background(), testUpstream, testRepo, digest)
		assert.ErrorIs(t, err, cache.ErrBlobNotFound)
	})

	t.Run("succeeds for non-existent blob", func(t *testing.T) {
		err := c.Delete(context.Background(), testUpstream, testRepo, "sha256:nonexistent")
		assert.NoError(t, err)
	})
}

func TestDiskBlobCache_InvalidDigest(t *testing.T) {
	c := newTestCache(t)
	content := []byte("test")

	invalidDigests := []string{
		"invalid", // no colon
		"sha256",  // no hex
		":abc123", // no algorithm
		"sha256:", // no hex value
		"",        // empty
	}

	for _, digest := range invalidDigests {
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}

		t.Run("Put with invalid digest: "+digest, func(t *testing.T) {
			putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
			require.Error(t, putErr)
			assertIsDigestInvalidError(t, putErr)
		})

		t.Run("Get with invalid digest: "+digest, func(t *testing.T) {
			_, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
			require.Error(t, getErr)
			assertIsDigestInvalidError(t, getErr)
		})

		t.Run("Head with invalid digest: "+digest, func(t *testing.T) {
			_, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
			require.Error(t, headErr)
			assertIsDigestInvalidError(t, headErr)
		})
	}

	// "sha256:abc:extra..." is valid for our cache because we split on first colon only.
	// However, the extra colons make it invalid for v1.NewHash, so we use a plain descriptor.
	t.Run("digest with extra colons is valid for cache storage", func(t *testing.T) {
		// This tests that our cache accepts digests with colons in the hex part.
		// v1.NewHash won't accept this, so we create a descriptor without a valid hash.
		digest := "sha256:" + strings.Repeat("a", 60) + ":ext"
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		// We can retrieve the blob content
		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		_ = reader.Close()
	})
}

func TestDiskBlobCache_ContentAddressable(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("5a2ed123")

	content1 := []byte("original content")
	content2 := []byte("updated content")
	desc1 := testDescriptor(digest, int64(len(content1)), types.DockerLayer)
	desc2 := testDescriptor(digest, int64(len(content2)), types.OCILayer)

	t.Run("same digest overwrites previous content and descriptor", func(t *testing.T) {
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc1, bytes.NewReader(content1))
		require.NoError(t, err)

		// Second put with same digest overwrites
		err = c.Put(context.Background(), testUpstream, testRepo, digest, desc2, bytes.NewReader(content2))
		require.NoError(t, err)

		// Should return the latest content
		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		retrieved, err := io.ReadAll(reader)
		require.NoError(t, err)
		reader.Close()
		assert.Equal(t, content2, retrieved)

		// Should return the latest descriptor
		gotDesc, err := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		assert.Equal(t, desc2.Size, gotDesc.Size)
		assert.Equal(t, desc2.MediaType, gotDesc.MediaType)
	})
}

func TestDiskBlobCache_LargeBlob(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("1a4eb10b123")

	// Create a 1MB blob
	size := 1024 * 1024
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 256)
	}
	desc := testDescriptor(digest, int64(size), types.DockerLayer)

	t.Run("handles large blob", func(t *testing.T) {
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		// Verify size via descriptor
		gotDesc, err := c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		assert.Equal(t, int64(size), gotDesc.Size)

		// Verify content
		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		defer reader.Close()

		retrieved, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, content, retrieved)
	})
}

func TestDiskBlobCache_SizeMismatch(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("512e1a7c123")
	content := []byte("actual content")

	t.Run("fails when descriptor size doesn't match actual", func(t *testing.T) {
		wrongSize := int64(len(content) + 100)
		desc := testDescriptor(digest, wrongSize, types.DockerLayer)
		putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.Error(t, putErr)
		assertIsSizeInvalidError(t, putErr)
	})

	t.Run("succeeds with zero size (no verification)", func(t *testing.T) {
		desc := testDescriptor(digest, 0, types.DockerLayer)
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		// Verify it was stored
		_, err = c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
	})
}

func TestDiskBlobCache_Overwrite(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("04e4123")

	content1 := []byte("original content")
	content2 := []byte("updated content with more data")
	desc1 := testDescriptor(digest, int64(len(content1)), types.DockerLayer)
	desc2 := testDescriptor(digest, int64(len(content2)), types.DockerLayer)

	t.Run("overwrites existing blob", func(t *testing.T) {
		// Put original
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc1, bytes.NewReader(content1))
		require.NoError(t, err)

		// Overwrite
		err = c.Put(context.Background(), testUpstream, testRepo, digest, desc2, bytes.NewReader(content2))
		require.NoError(t, err)

		// Verify new content
		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		defer reader.Close()

		retrieved, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, content2, retrieved)
	})
}

func TestDiskBlobCache_PathTraversalPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := disk.NewBlobCache(&disk.BlobCacheOptions{
		BasePath: tmpDir,
	})
	require.NoError(t, err)

	content := []byte("safe content")

	t.Run("sanitizes path traversal in digest algorithm", func(t *testing.T) {
		digest := "../../../etc:passwd"
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}
		putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, putErr)

		// Verify the blob is inside the cache directory with sanitized path
		// "../../../etc" becomes "etc" (slashes and dots removed)
		info, statErr := os.Stat(filepath.Join(tmpDir, "etc", "passwd"))
		require.NoError(t, statErr)
		assert.False(t, info.IsDir())

		// Verify we can retrieve the content
		reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		_ = reader.Close()
	})

	t.Run("sanitizes path traversal in digest hex", func(t *testing.T) {
		digest := "sha256:../../../tmp/test"
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}
		putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, putErr)

		// Verify the blob is inside the cache directory with sanitized path
		// "../../../tmp/test" becomes "tmptest" (slashes and dots removed)
		info, statErr := os.Stat(filepath.Join(tmpDir, "sha256", "tmptest"))
		require.NoError(t, statErr)
		assert.False(t, info.IsDir())

		// Verify we can retrieve the content
		reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		_ = reader.Close()
	})

	t.Run("removes all slashes from digest components", func(t *testing.T) {
		digest := "sha/256:abc/def/ghi"
		desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}
		putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, putErr)

		// Verify slashes are stripped: "sha/256" -> "sha256", "abc/def/ghi" -> "abcdefghi"
		info, statErr := os.Stat(filepath.Join(tmpDir, "sha256", "abcdefghi"))
		require.NoError(t, statErr)
		assert.False(t, info.IsDir())

		// Verify we can retrieve the content
		reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, getErr)
		_ = reader.Close()
	})
}

func TestDiskBlobCache_DifferentAlgorithms(t *testing.T) {
	c := newTestCache(t)

	// Test different algorithm prefixes with their correct hex lengths.
	// Our cache supports any algorithm prefix; we just verify basic functionality.
	tests := []struct {
		alg    string
		hexLen int
	}{
		{"sha256", 64},
		{"sha384", 96},
		{"sha512", 128},
	}

	for _, tt := range tests {
		t.Run("supports "+tt.alg+" digest", func(t *testing.T) {
			digest := tt.alg + ":" + strings.Repeat("a", tt.hexLen)
			content := []byte("content for " + tt.alg)
			// Use a plain descriptor since v1.NewHash only supports sha256 with 64 chars
			desc := &v1.Descriptor{Size: int64(len(content)), MediaType: types.DockerLayer}

			err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
			require.NoError(t, err)

			// Verify we can retrieve the content
			reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, err)
			retrieved, err := io.ReadAll(reader)
			require.NoError(t, err)
			_ = reader.Close()
			assert.Equal(t, content, retrieved)
		})
	}
}

func TestDiskBlobCache_ConcurrentAccess(t *testing.T) {
	c := newTestCache(t)

	// hexChars contains only valid hex characters for generating test digests
	hexChars := []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'}

	t.Run("concurrent writes to different blobs", func(t *testing.T) {
		const numGoroutines = 10
		done := make(chan bool, numGoroutines)

		for i := range numGoroutines {
			go func(idx int) {
				// Use valid hex char for the digest
				hexChar := hexChars[idx%len(hexChars)]
				digest := "sha256:" + strings.Repeat(string(hexChar), 64)
				content := []byte(strings.Repeat("x", 1000))
				desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

				putErr := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
				assert.NoError(t, putErr)

				_, headErr := c.Head(context.Background(), testUpstream, testRepo, digest)
				assert.NoError(t, headErr)

				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for range numGoroutines {
			<-done
		}

		// Verify all blobs exist
		for i := range numGoroutines {
			hexChar := hexChars[i%len(hexChars)]
			digest := "sha256:" + strings.Repeat(string(hexChar), 64)
			_, err := c.Head(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, err, "blob %d should exist", i)
		}
	})

	t.Run("concurrent reads and writes to same blob", func(t *testing.T) {
		digest := "sha256:" + strings.Repeat("f", 64)
		content := []byte("initial content")
		desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

		// First, create the blob
		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		const numReaders = 5
		const numWriters = 3
		done := make(chan bool, numReaders+numWriters)

		// Start readers
		for range numReaders {
			go func() {
				for range 10 {
					reader, getErr := c.Get(context.Background(), testUpstream, testRepo, digest)
					if getErr == nil {
						_, _ = io.ReadAll(reader)
						_ = reader.Close()
					}
				}
				done <- true
			}()
		}

		// Start writers
		for i := range numWriters {
			go func(idx int) {
				// Use valid hex char for content
				hexChar := hexChars[idx%len(hexChars)]
				newContent := []byte(strings.Repeat(string(hexChar), 100))
				newDesc := testDescriptor(digest, int64(len(newContent)), types.DockerLayer)
				for range 5 {
					_ = c.Put(
						context.Background(),
						testUpstream,
						testRepo,
						digest,
						newDesc,
						bytes.NewReader(newContent),
					)
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for range numReaders + numWriters {
			<-done
		}

		// Verify blob still exists and is readable
		_, err = c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		reader, err := c.Get(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		_ = reader.Close()
		assert.NotEmpty(t, data)
	})
}

func TestDiskBlobCache_DescriptorMediaTypes(t *testing.T) {
	c := newTestCache(t)

	mediaTypes := []types.MediaType{
		types.DockerLayer,
		types.DockerUncompressedLayer,
		types.OCILayer,
		types.OCIUncompressedLayer,
		types.DockerConfigJSON,
		types.OCIContentDescriptor,
	}

	for _, mt := range mediaTypes {
		t.Run("preserves media type "+string(mt), func(t *testing.T) {
			digest := "sha256:" + strings.Repeat(string(mt[0]), 64)
			content := []byte("content for " + string(mt))
			desc := testDescriptor(digest, int64(len(content)), mt)

			err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
			require.NoError(t, err)

			gotDesc, err := c.Head(context.Background(), testUpstream, testRepo, digest)
			require.NoError(t, err)
			assert.Equal(t, mt, gotDesc.MediaType)
		})
	}
}

func TestDiskBlobCache_List(t *testing.T) {
	t.Run("returns empty list when no blobs cached", func(t *testing.T) {
		c := newTestCache(t)
		blobs, err := c.List(context.Background())
		require.NoError(t, err)
		assert.Empty(t, blobs)
	})

	t.Run("returns all cached blobs", func(t *testing.T) {
		c := newTestCache(t)

		digests := []string{
			testDigest("1111111111111111"),
			testDigest("2222222222222222"),
			testDigest("3333333333333333"),
		}

		for _, digest := range digests {
			content := []byte("content for " + digest[:20])
			desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)
			err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
			require.NoError(t, err)
		}

		blobs, err := c.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, blobs, 3)

		// Collect returned digests for order-independent comparison
		foundDigests := make(map[string]bool)
		for _, b := range blobs {
			foundDigests[b.Digest] = true
			assert.NotEmpty(t, b.Path)
			assert.Positive(t, b.Size)
			assert.Equal(t, string(types.DockerLayer), b.MediaType)
		}

		for _, digest := range digests {
			assert.True(t, foundDigests[digest], "expected digest %s not found", digest)
		}
	})

	t.Run("excludes deleted blobs", func(t *testing.T) {
		c := newTestCache(t)

		digest1 := testDigest("aaaa11111111")
		digest2 := testDigest("bbbb22222222")

		for _, digest := range []string{digest1, digest2} {
			content := []byte("content for " + digest[:20])
			desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)
			err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
			require.NoError(t, err)
		}

		// Delete one
		err := c.Delete(context.Background(), testUpstream, testRepo, digest1)
		require.NoError(t, err)

		blobs, err := c.List(context.Background())
		require.NoError(t, err)
		assert.Len(t, blobs, 1)
		assert.Equal(t, digest2, blobs[0].Digest)
	})

	t.Run("returns error with cancelled context", func(t *testing.T) {
		c := newTestCache(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := c.List(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("preserves blob metadata", func(t *testing.T) {
		c := newTestCache(t)
		digest := testDigest("def0123456")
		content := []byte("metadata test content")
		desc := testDescriptor(digest, int64(len(content)), types.OCILayer)

		err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
		require.NoError(t, err)

		// Call Head first to ensure the descriptor is loaded into the ristretto cache,
		// since ristretto's Set is eventually consistent.
		_, err = c.Head(context.Background(), testUpstream, testRepo, digest)
		require.NoError(t, err)

		blobs, err := c.List(context.Background())
		require.NoError(t, err)
		require.Len(t, blobs, 1)

		assert.Equal(t, digest, blobs[0].Digest)
		assert.Equal(t, int64(len(content)), blobs[0].Size)
		assert.Equal(t, string(types.OCILayer), blobs[0].MediaType)
		assert.NotEmpty(t, blobs[0].Path)
	})
}

// assertIsDigestInvalidError checks if an error is an HTTPError with DIGEST_INVALID code.
func assertIsDigestInvalidError(t *testing.T, err error) {
	t.Helper()
	var httpErr *httptk.HTTPError
	if !errors.As(err, &httpErr) {
		t.Errorf("expected *httptk.HTTPError, got %T: %v", err, err)
		return
	}
	if !httpErr.IsDigestInvalid() {
		t.Errorf("expected DIGEST_INVALID error code, got %s", httpErr.Code)
	}
}

// assertIsSizeInvalidError checks if an error is an HTTPError with SIZE_INVALID code.
func assertIsSizeInvalidError(t *testing.T, err error) {
	t.Helper()
	var httpErr *httptk.HTTPError
	if !errors.As(err, &httpErr) {
		t.Errorf("expected *httptk.HTTPError, got %T: %v", err, err)
		return
	}
	if !httpErr.IsSizeInvalid() {
		t.Errorf("expected SIZE_INVALID error code, got %s", httpErr.Code)
	}
}

func TestDiskBlobCache_ContextCancellation(t *testing.T) {
	c := newTestCache(t)
	digest := testDigest("ctxcancel123")
	content := []byte("context cancellation test")
	desc := testDescriptor(digest, int64(len(content)), types.DockerLayer)

	// First store a blob so we can test Get and Head with cancelled context
	err := c.Put(context.Background(), testUpstream, testRepo, digest, desc, bytes.NewReader(content))
	require.NoError(t, err)

	t.Run("Head returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, headErr := c.Head(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, headErr, context.Canceled)
	})

	t.Run("Get returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, getErr := c.Get(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, getErr, context.Canceled)
	})

	t.Run("Put returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		newDigest := testDigest("ctxcancel456")
		putErr := c.Put(ctx, testUpstream, testRepo, newDigest, desc, bytes.NewReader(content))
		assert.ErrorIs(t, putErr, context.Canceled)
	})

	t.Run("Delete returns error with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		deleteErr := c.Delete(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, deleteErr, context.Canceled)
	})

	t.Run("Head returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		_, headErr := c.Head(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, headErr, context.DeadlineExceeded)
	})

	t.Run("Get returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		_, getErr := c.Get(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, getErr, context.DeadlineExceeded)
	})

	t.Run("Put returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		newDigest := testDigest("ctxdeadline789")
		putErr := c.Put(ctx, testUpstream, testRepo, newDigest, desc, bytes.NewReader(content))
		assert.ErrorIs(t, putErr, context.DeadlineExceeded)
	})

	t.Run("Delete returns error with deadline exceeded context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		deleteErr := c.Delete(ctx, testUpstream, testRepo, digest)
		assert.ErrorIs(t, deleteErr, context.DeadlineExceeded)
	})
}
