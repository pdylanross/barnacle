package imagegen

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/stream"
)

const (
	randNameLen     = 8
	filePerm        = int64(0644)
	dirPerm         = int64(0755)
	chunkSize       = 1024 * 1024 // 1MB chunks
	randInt64Bytes  = 8
	smallLayerMin   = 1 * 1024 * 1024    // 1MB
	smallLayerMax   = 100 * 1024 * 1024  // 100MB
	largeLayerMin   = 100 * 1024 * 1024  // 100MB
	largeLayerMax   = 1024 * 1024 * 1024 // 1GB
	weightThreshold = 204                // 204/256 ≈ 80%
	bitShift56      = 56
)

// RandomLayer creates a new layer with random binary content of the specified size.
// The layer contains a single file with random data at /data/<random-name>.bin.
func RandomLayer(size int64) (v1.Layer, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Generate random filename
	randName := make([]byte, randNameLen)
	if _, err := rand.Read(randName); err != nil {
		return nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	filename := fmt.Sprintf("data/%x.bin", randName)

	// Create the directory entry
	dirHeader := &tar.Header{
		Name:     "data/",
		Mode:     dirPerm,
		Typeflag: tar.TypeDir,
		ModTime:  time.Now(),
	}
	if err := tw.WriteHeader(dirHeader); err != nil {
		return nil, fmt.Errorf("failed to write dir header: %w", err)
	}

	// Create the file header
	header := &tar.Header{
		Name:    filename,
		Mode:    filePerm,
		Size:    size,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("failed to write file header: %w", err)
	}

	// Write random content in chunks to avoid memory issues with large files
	remaining := size
	chunk := make([]byte, chunkSize)

	for remaining > 0 {
		toWrite := chunkSize
		if remaining < int64(chunkSize) {
			toWrite = int(remaining)
			chunk = make([]byte, toWrite)
		}

		if _, err := rand.Read(chunk[:toWrite]); err != nil {
			return nil, fmt.Errorf("failed to generate random content: %w", err)
		}

		if _, err := tw.Write(chunk[:toWrite]); err != nil {
			return nil, fmt.Errorf("failed to write content: %w", err)
		}

		remaining -= int64(toWrite)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Create the layer from the tar archive using stream.NewLayer
	// This avoids writing temporary files to disk
	layer := stream.NewLayer(io.NopCloser(bytes.NewReader(buf.Bytes())))

	return layer, nil
}

// RandomLayerStream creates a layer from a streaming random reader to handle very large layers.
// This is more memory-efficient for layers > 100MB.
func RandomLayerStream(size int64) (v1.Layer, error) {
	pr, pw := io.Pipe()

	go func() {
		tw := tar.NewWriter(pw)

		// Generate random filename
		randName := make([]byte, randNameLen)
		_, _ = rand.Read(randName)
		filename := fmt.Sprintf("data/%x.bin", randName)

		// Create the directory entry
		_ = tw.WriteHeader(&tar.Header{
			Name:     "data/",
			Mode:     dirPerm,
			Typeflag: tar.TypeDir,
			ModTime:  time.Now(),
		})

		// Create the file header
		_ = tw.WriteHeader(&tar.Header{
			Name:    filename,
			Mode:    filePerm,
			Size:    size,
			ModTime: time.Now(),
		})

		// Write random content in chunks
		remaining := size
		chunk := make([]byte, chunkSize)

		for remaining > 0 {
			toWrite := min(remaining, int64(chunkSize))

			_, _ = rand.Read(chunk[:toWrite])
			_, _ = tw.Write(chunk[:toWrite])

			remaining -= toWrite
		}

		_ = tw.Close()
		_ = pw.Close()
	}()

	// Use stream.NewLayer to avoid writing temporary files to disk
	return stream.NewLayer(pr), nil
}

// WeightedRandomSize returns a random layer size following the weighted distribution:
// 80% of layers are 1-100Mi, 20% are up to 1Gi.
func WeightedRandomSize() int64 {
	weightByte := make([]byte, 1)
	_, _ = rand.Read(weightByte)

	if weightByte[0] < weightThreshold {
		return randomInt64Range(smallLayerMin, smallLayerMax)
	}

	return randomInt64Range(largeLayerMin, largeLayerMax)
}

// randomInt64Range returns a random int64 between lo and hi (inclusive).
func randomInt64Range(lo, hi int64) int64 {
	if lo >= hi {
		return lo
	}

	rangeSize := hi - lo + 1
	randBytes := make([]byte, randInt64Bytes)
	_, _ = rand.Read(randBytes)

	randVal := uint64(randBytes[0]) | uint64(randBytes[1])<<8 | uint64(randBytes[2])<<16 |
		uint64(randBytes[3])<<24 | uint64(randBytes[4])<<32 | uint64(randBytes[5])<<40 |
		uint64(randBytes[6])<<48 | uint64(randBytes[7])<<bitShift56

	return lo + int64(randVal%uint64(rangeSize)) //nolint:gosec // rangeSize is always positive here
}
