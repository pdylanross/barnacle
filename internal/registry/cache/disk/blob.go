// Package disk provides a disk-based implementation of the blob cache.
// It stores blobs as files on the local filesystem, organized by digest.
// Since container blobs are content-addressable, the same blob can be shared
// across different upstreams and repositories.
//
// The directory structure is:
//
//	<basePath>/<algorithm>/<hex>
//	<basePath>/<algorithm>/<hex>.descriptor.json
//
// For example, a blob with digest sha256:abc123 would be stored at:
//
//	/tmp/barnacle/sha256/abc123
//	/tmp/barnacle/sha256/abc123.descriptor.json
//
// The cache uses atomic writes (write to temp file, then rename) to prevent
// corruption from incomplete writes or concurrent access. Descriptors are
// also kept in an in-memory ristretto cache for fast lookups.
package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgraph-io/ristretto/v2"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pdylanross/barnacle/internal/registry/cache"
	"github.com/pdylanross/barnacle/internal/tk/httptk"
	"go.uber.org/zap"
)

// DefaultBasePath is the default location for the disk blob cache.
const DefaultBasePath = "/tmp/barnacle"

// Default cache settings matching the manifest cache defaults.
const (
	// DefaultDescriptorLimit is the maximum number of descriptors to cache in memory.
	DefaultDescriptorLimit = 10000

	// digestSplitParts is the expected number of parts when splitting a digest on ":".
	digestSplitParts = 2

	// descriptorFileSuffix is the suffix for descriptor JSON files.
	descriptorFileSuffix = ".descriptor.json"

	// ristrettoNumCountersMultiplier is the multiplier used to calculate NumCounters
	// from the max cost. Ristretto recommends 10x the max number of items.
	ristrettoNumCountersMultiplier = 10

	// ristrettoBufferItems is the number of keys per Get buffer in ristretto.
	ristrettoBufferItems = 64

	// descriptorCost is the fixed cost assigned to each descriptor entry in the cache.
	descriptorCost int64 = 1
)

// BlobCacheOptions configures the disk blob cache.
type BlobCacheOptions struct {
	// BasePath is the root directory for blob storage.
	// Defaults to DefaultBasePath if empty.
	BasePath string

	// DescriptorLimit is the maximum number of descriptors to cache in memory.
	// Defaults to DefaultDescriptorLimit if zero.
	DescriptorLimit int

	// Logger is the logger to use for debug logging.
	// If nil, a no-op logger is used.
	Logger *zap.Logger
}

// NewBlobCache creates a new disk-based blob cache with the given options.
// The cache stores blobs as files on the local filesystem and maintains
// an in-memory ristretto cache for descriptor lookups.
//
// On initialization, the cache scans the disk to discover existing cached blobs
// and populates the items map accordingly.
//
// Returns an error if the base directory cannot be created or if the
// ristretto cache fails to initialize.
func NewBlobCache(opts *BlobCacheOptions) (cache.BlobCache, error) {
	basePath := opts.BasePath
	if basePath == "" {
		basePath = DefaultBasePath
	}

	descriptorLimit := opts.DescriptorLimit
	if descriptorLimit == 0 {
		descriptorLimit = DefaultDescriptorLimit
	}

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger = logger.Named("diskBlobCache")

	logger.Debug("initializing disk blob cache",
		zap.String("basePath", basePath),
		zap.Int("descriptorLimit", descriptorLimit))

	// Ensure the base directory exists
	if err := os.MkdirAll(basePath, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create ristretto cache for descriptors
	descriptorCacheOptions := ristretto.Config[string, *v1.Descriptor]{
		MaxCost:     int64(descriptorLimit),
		NumCounters: int64(ristrettoNumCountersMultiplier * descriptorLimit),
		BufferItems: ristrettoBufferItems,
	}

	descriptorCache, err := ristretto.NewCache(&descriptorCacheOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create descriptor cache: %w", err)
	}

	c := &diskBlobCache{
		basePath:        basePath,
		items:           make(map[string]*diskCachedItem),
		descriptorCache: descriptorCache,
		logger:          logger,
	}

	// Scan disk to populate items map with existing blobs
	if err = c.scanDisk(); err != nil {
		return nil, fmt.Errorf("failed to scan cache directory: %w", err)
	}

	logger.Info("disk blob cache initialized",
		zap.String("basePath", basePath),
		zap.Int("existingItems", len(c.items)))

	return c, nil
}

// diskCachedItem represents a cached blob with its metadata and per-item lock.
// The lock ensures thread-safe access to individual blobs without blocking
// operations on other blobs.
type diskCachedItem struct {
	// path is the filesystem path to this blob.
	path string
	// mu protects operations on this specific blob.
	mu sync.RWMutex
}

// diskBlobCache implements cache.BlobCache using the local filesystem.
// It uses per-item locking to allow concurrent access to different blobs
// and a ristretto cache for fast descriptor lookups.
type diskBlobCache struct {
	basePath        string
	items           map[string]*diskCachedItem
	itemsMu         sync.RWMutex // protects the items map
	descriptorCache *ristretto.Cache[string, *v1.Descriptor]
	logger          *zap.Logger
}

// scanDisk walks the cache directory and populates the items map with existing blobs.
// This is called during cache initialization to restore state from disk.
func (d *diskBlobCache) scanDisk() error {
	d.logger.Debug("scanning disk for existing blobs", zap.String("basePath", d.basePath))

	// Read algorithm directories (e.g., sha256, sha384, sha512)
	entries, err := os.ReadDir(d.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			d.logger.Debug("cache directory does not exist, starting with empty cache")
			return nil // Empty cache, nothing to scan
		}
		return err
	}

	scannedCount := 0
	for _, algEntry := range entries {
		if !algEntry.IsDir() {
			continue
		}

		algPath := filepath.Join(d.basePath, algEntry.Name())
		blobEntries, readErr := os.ReadDir(algPath)
		if readErr != nil {
			d.logger.Debug("skipping unreadable directory",
				zap.String("path", algPath),
				zap.Error(readErr))
			continue // Skip directories we can't read
		}

		algorithmBlobCount := 0
		for _, blobEntry := range blobEntries {
			// Skip descriptor files and directories
			if blobEntry.IsDir() || strings.HasSuffix(blobEntry.Name(), descriptorFileSuffix) {
				continue
			}

			// Skip temp files
			if strings.HasPrefix(blobEntry.Name(), ".") {
				continue
			}

			blobPath := filepath.Join(algPath, blobEntry.Name())
			d.items[blobPath] = &diskCachedItem{path: blobPath}
			algorithmBlobCount++
			scannedCount++
		}

		if algorithmBlobCount > 0 {
			d.logger.Debug("scanned algorithm directory",
				zap.String("algorithm", algEntry.Name()),
				zap.Int("blobCount", algorithmBlobCount))
		}
	}

	d.logger.Debug("disk scan complete", zap.Int("totalBlobs", scannedCount))
	return nil
}

// getOrCreateItem returns the cached item for the given path, creating it if necessary.
// This method is safe for concurrent access.
func (d *diskBlobCache) getOrCreateItem(path string) *diskCachedItem {
	// Try read lock first for the common case where item exists
	d.itemsMu.RLock()
	item, ok := d.items[path]
	d.itemsMu.RUnlock()

	if ok {
		return item
	}

	// Item doesn't exist, acquire write lock to create it
	d.itemsMu.Lock()
	defer d.itemsMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if item, ok = d.items[path]; ok {
		return item
	}

	item = &diskCachedItem{path: path}
	d.items[path] = item
	return item
}

// Head returns the descriptor for a cached blob.
// It first checks the in-memory ristretto cache, then falls back to disk.
// The upstream and repo parameters are not used for storage (blobs are content-addressable)
// but are part of the interface for routing purposes.
// Returns ErrBlobNotFound if the blob is not cached.
func (d *diskBlobCache) Head(ctx context.Context, _, _, digest string) (*v1.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := d.blobPath(digest)
	if err != nil {
		d.logger.Debug("Head: invalid digest", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	// Check ristretto cache first
	if desc, ok := d.descriptorCache.Get(digest); ok {
		d.logger.Debug("Head: descriptor cache hit", zap.String("digest", digest))
		return desc, nil
	}

	item := d.getOrCreateItem(path)
	item.mu.RLock()
	defer item.mu.RUnlock()

	// Check if blob exists
	if _, err = os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			d.logger.Debug("Head: blob not found", zap.String("digest", digest))
			return nil, cache.ErrBlobNotFound
		}
		d.logger.Debug("Head: stat error", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	// Load descriptor from disk
	desc, err := d.loadDescriptor(path)
	if err != nil {
		d.logger.Debug("Head: failed to load descriptor",
			zap.String("digest", digest),
			zap.Error(err))
		return nil, err
	}

	// Cache the descriptor in memory
	d.descriptorCache.Set(digest, desc, descriptorCost)
	d.logger.Debug("Head: loaded from disk and cached",
		zap.String("digest", digest),
		zap.Int64("size", desc.Size))

	return desc, nil
}

// Get retrieves a cached blob by digest.
// The returned [io.ReadCloser] must be closed by the caller.
// The upstream and repo parameters are not used for storage (blobs are content-addressable)
// but are part of the interface for routing purposes.
// Note: The per-item lock is only held during file open, not during read,
// since atomic writes (temp file + rename) ensure file integrity.
func (d *diskBlobCache) Get(ctx context.Context, _, _, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := d.blobPath(digest)
	if err != nil {
		d.logger.Debug("Get: invalid digest", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	item := d.getOrCreateItem(path)
	item.mu.RLock()
	defer item.mu.RUnlock()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			d.logger.Debug("Get: blob not found", zap.String("digest", digest))
			return nil, cache.ErrBlobNotFound
		}
		d.logger.Debug("Get: open error", zap.String("digest", digest), zap.Error(err))
		return nil, err
	}

	d.logger.Debug("Get: returning cached blob", zap.String("digest", digest))
	return file, nil
}

// Put stores a blob and its descriptor in the cache.
// The upstream and repo parameters are not used for storage (blobs are content-addressable)
// but are part of the interface for routing purposes.
func (d *diskBlobCache) Put(
	ctx context.Context,
	_, _, digest string,
	descriptor *v1.Descriptor,
	content io.Reader,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	d.logger.Debug("Put: storing blob",
		zap.String("digest", digest),
		zap.Int64("expectedSize", descriptor.Size))

	path, err := d.blobPath(digest)
	if err != nil {
		d.logger.Debug("Put: invalid digest", zap.String("digest", digest), zap.Error(err))
		return err
	}

	item := d.getOrCreateItem(path)
	item.mu.Lock()
	defer item.mu.Unlock()

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create blob directory: %w", err)
	}

	// Write blob content to a temporary file first for atomic operation
	tmpFile, err := os.CreateTemp(dir, ".blob-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Copy content to temp file
	written, err := io.Copy(tmpFile, content)
	if err != nil {
		return fmt.Errorf("failed to write blob content: %w", err)
	}

	// Verify size if provided in descriptor
	if descriptor.Size > 0 && written != descriptor.Size {
		d.logger.Debug("Put: size mismatch",
			zap.String("digest", digest),
			zap.Int64("expected", descriptor.Size),
			zap.Int64("actual", written))
		return httptk.ErrSizeInvalid(fmt.Sprintf(
			"blob size mismatch: expected %d, got %d",
			descriptor.Size,
			written,
		))
	}

	// Close the file before renaming
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename for blob
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to finalize blob: %w", err)
	}

	// Write descriptor to disk
	if err = d.saveDescriptor(path, descriptor); err != nil {
		// Try to clean up the blob file if descriptor write fails
		_ = os.Remove(path)
		return fmt.Errorf("failed to save descriptor: %w", err)
	}

	// Cache the descriptor in memory
	d.descriptorCache.Set(digest, descriptor, descriptorCost)

	d.logger.Debug("Put: blob stored successfully",
		zap.String("digest", digest),
		zap.Int64("size", written))

	success = true
	return nil
}

// Delete removes a blob and its descriptor from the cache.
// The upstream and repo parameters are not used for storage (blobs are content-addressable)
// but are part of the interface for routing purposes.
func (d *diskBlobCache) Delete(ctx context.Context, _, _, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	d.logger.Debug("Delete: removing blob", zap.String("digest", digest))

	path, err := d.blobPath(digest)
	if err != nil {
		d.logger.Debug("Delete: invalid digest", zap.String("digest", digest), zap.Error(err))
		return err
	}

	item := d.getOrCreateItem(path)
	item.mu.Lock()
	defer item.mu.Unlock()

	// Remove blob file
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		d.logger.Debug("Delete: failed to remove blob file",
			zap.String("digest", digest),
			zap.Error(err))
		return err
	}

	// Remove descriptor file
	descPath := path + descriptorFileSuffix
	err = os.Remove(descPath)
	if err != nil && !os.IsNotExist(err) {
		d.logger.Debug("Delete: failed to remove descriptor file",
			zap.String("digest", digest),
			zap.Error(err))
		return err
	}

	// Remove from memory cache
	d.descriptorCache.Del(digest)

	// Remove item from map
	d.itemsMu.Lock()
	delete(d.items, path)
	d.itemsMu.Unlock()

	d.logger.Debug("Delete: blob removed successfully", zap.String("digest", digest))
	return nil
}

// List returns information about all blobs in the disk cache.
// It iterates the items map under a read lock and loads descriptors
// to populate size and media type information.
func (d *diskBlobCache) List(ctx context.Context) ([]cache.BlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	d.itemsMu.RLock()
	paths := make([]string, 0, len(d.items))
	for path := range d.items {
		paths = append(paths, path)
	}
	d.itemsMu.RUnlock()

	blobs := make([]cache.BlobInfo, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Reconstruct digest from path: basePath/algorithm/hex -> algorithm:hex
		dir := filepath.Dir(path)
		algorithm := filepath.Base(dir)
		hex := filepath.Base(path)
		digest := algorithm + ":" + hex

		info := cache.BlobInfo{
			Digest: digest,
			Path:   path,
		}

		// Try to load descriptor for size and media type
		if desc, ok := d.descriptorCache.Get(digest); ok {
			info.Size = desc.Size
			info.MediaType = string(desc.MediaType)
		} else if diskDesc, loadErr := d.loadDescriptor(path); loadErr == nil {
			info.Size = diskDesc.Size
			info.MediaType = string(diskDesc.MediaType)
			d.descriptorCache.Set(digest, diskDesc, descriptorCost)
		}

		blobs = append(blobs, info)
	}

	return blobs, nil
}

// blobPath constructs the filesystem path for a blob.
// The path format is: basePath/algorithm/hex.
func (d *diskBlobCache) blobPath(digest string) (string, error) {
	algorithm, hex, err := parseDigest(digest)
	if err != nil {
		return "", err
	}

	// Sanitize path components to prevent directory traversal
	algorithm = sanitizePath(algorithm)
	hex = sanitizePath(hex)

	return filepath.Join(d.basePath, algorithm, hex), nil
}

// loadDescriptor reads a descriptor from disk.
func (d *diskBlobCache) loadDescriptor(blobPath string) (*v1.Descriptor, error) {
	descPath := blobPath + descriptorFileSuffix

	data, err := os.ReadFile(descPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, cache.ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to read descriptor: %w", err)
	}

	var desc v1.Descriptor
	if err = json.Unmarshal(data, &desc); err != nil {
		return nil, fmt.Errorf("failed to parse descriptor: %w", err)
	}

	return &desc, nil
}

// saveDescriptor writes a descriptor to disk atomically.
func (d *diskBlobCache) saveDescriptor(blobPath string, descriptor *v1.Descriptor) error {
	descPath := blobPath + descriptorFileSuffix
	dir := filepath.Dir(descPath)

	data, err := json.Marshal(descriptor)
	if err != nil {
		return fmt.Errorf("failed to marshal descriptor: %w", err)
	}

	// Write to temp file first for atomic operation
	tmpFile, err := os.CreateTemp(dir, ".desc-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up on error
	success := false
	defer func() {
		if !success {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write descriptor: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err = os.Rename(tmpPath, descPath); err != nil {
		return fmt.Errorf("failed to finalize descriptor: %w", err)
	}

	success = true
	return nil
}

// parseDigest splits a digest into its algorithm and hex components.
// Expected format: "algorithm:hex" (e.g., "sha256:abc123...").
func parseDigest(digest string) (string, string, error) {
	parts := strings.SplitN(digest, ":", digestSplitParts)
	if len(parts) != digestSplitParts {
		return "", "", httptk.ErrDigestInvalid(
			fmt.Sprintf("expected 'algorithm:hex', got %q", digest),
		)
	}

	algorithm := parts[0]
	hex := parts[1]

	if algorithm == "" || hex == "" {
		return "", "", httptk.ErrDigestInvalid(
			fmt.Sprintf("empty algorithm or hex in %q", digest),
		)
	}

	return algorithm, hex, nil
}

// sanitizePath removes potentially dangerous path components.
// It removes all slashes and prevents directory traversal.
func sanitizePath(s string) string {
	// Remove any path traversal attempts
	s = strings.ReplaceAll(s, "..", "")
	// Remove all slashes
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "\\", "")
	return s
}
