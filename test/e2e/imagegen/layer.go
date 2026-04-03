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

// RandomLayer creates a new layer with random binary content of the specified size.
// The layer contains a single file with random data at /data/<random-name>.bin
func RandomLayer(size int64) (v1.Layer, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Generate random filename
	randName := make([]byte, 8)
	if _, err := rand.Read(randName); err != nil {
		return nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	filename := fmt.Sprintf("data/%x.bin", randName)

	// Create the directory entry
	dirHeader := &tar.Header{
		Name:     "data/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
		ModTime:  time.Now(),
	}
	if err := tw.WriteHeader(dirHeader); err != nil {
		return nil, fmt.Errorf("failed to write dir header: %w", err)
	}

	// Create the file header
	header := &tar.Header{
		Name:    filename,
		Mode:    0644,
		Size:    size,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("failed to write file header: %w", err)
	}

	// Write random content in chunks to avoid memory issues with large files
	const chunkSize = 1024 * 1024 // 1MB chunks
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
		randName := make([]byte, 8)
		rand.Read(randName) //nolint:errcheck
		filename := fmt.Sprintf("data/%x.bin", randName)

		// Create the directory entry
		tw.WriteHeader(&tar.Header{ //nolint:errcheck
			Name:     "data/",
			Mode:     0755,
			Typeflag: tar.TypeDir,
			ModTime:  time.Now(),
		})

		// Create the file header
		tw.WriteHeader(&tar.Header{ //nolint:errcheck
			Name:    filename,
			Mode:    0644,
			Size:    size,
			ModTime: time.Now(),
		})

		// Write random content in chunks
		const chunkSize = 1024 * 1024 // 1MB chunks
		remaining := size
		chunk := make([]byte, chunkSize)

		for remaining > 0 {
			toWrite := int64(chunkSize)
			if remaining < toWrite {
				toWrite = remaining
			}

			rand.Read(chunk[:toWrite]) //nolint:errcheck
			tw.Write(chunk[:toWrite])  //nolint:errcheck

			remaining -= toWrite
		}

		tw.Close()
		pw.Close()
	}()

	// Use stream.NewLayer to avoid writing temporary files to disk
	return stream.NewLayer(pr), nil
}

// WeightedRandomSize returns a random layer size following the weighted distribution:
// 80% of layers are 1-100Mi, 20% are up to 1Gi
func WeightedRandomSize() int64 {
	// Generate random byte for weighting decision
	weightByte := make([]byte, 1)
	rand.Read(weightByte) //nolint:errcheck

	// Use the random byte to determine weight (0-255)
	// 80% = 204/256, so if < 204, use small size
	if weightByte[0] < 204 {
		// Small layer: 1MB to 100MB
		return randomInt64(1*1024*1024, 100*1024*1024)
	}

	// Large layer: 100MB to 1GB
	return randomInt64(100*1024*1024, 1024*1024*1024)
}

// randomInt64 returns a random int64 between min and max (inclusive)
func randomInt64(min, max int64) int64 {
	if min >= max {
		return min
	}

	rangeSize := max - min + 1
	randBytes := make([]byte, 8)
	rand.Read(randBytes) //nolint:errcheck

	// Convert to uint64 and scale to range
	randVal := uint64(randBytes[0]) | uint64(randBytes[1])<<8 | uint64(randBytes[2])<<16 |
		uint64(randBytes[3])<<24 | uint64(randBytes[4])<<32 | uint64(randBytes[5])<<40 |
		uint64(randBytes[6])<<48 | uint64(randBytes[7])<<56

	return min + int64(randVal%uint64(rangeSize))
}
