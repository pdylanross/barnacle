package imagegen

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.uber.org/zap"
)

// Config holds the configuration for image generation.
type Config struct {
	// BaseImage is the base image to use for generation (e.g., "alpine:latest")
	BaseImage string

	// TargetRegistry is where generated images will be pushed
	TargetRegistry string

	// NumVariants is the total number of image variants to generate
	NumVariants int

	// MinLayers is the minimum number of layers to add per image
	MinLayers int

	// MaxLayers is the maximum number of layers to add per image
	MaxLayers int

	// LayerSharingPercent is the percentage of images that should be based on
	// previously generated images instead of the base image (0-100)
	LayerSharingPercent int

	// Concurrency is the number of concurrent image generations
	Concurrency int

	// Insecure allows pushing to insecure registries
	Insecure bool
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		BaseImage:           "alpine:latest",
		TargetRegistry:      "localhost:5000",
		NumVariants:         100,
		MinLayers:           1,
		MaxLayers:           4,
		LayerSharingPercent: 50,
		Concurrency:         4,
		Insecure:            true,
	}
}

// ImageManifest represents the manifest of generated images.
type ImageManifest struct {
	GeneratedAt time.Time     `json:"generated_at"`
	BaseImage   string        `json:"base_image"`
	Images      []ImageEntry  `json:"images"`
	Statistics  ManifestStats `json:"statistics"`
}

// ImageEntry represents a single generated image.
type ImageEntry struct {
	Name       string  `json:"name"`
	Tag        string  `json:"tag"`
	Reference  string  `json:"reference"`
	Digest     string  `json:"digest"`
	NumLayers  int     `json:"num_layers"`
	TotalSize  int64   `json:"total_size"`
	ParentRef  string  `json:"parent_ref,omitempty"`
	LayerSizes []int64 `json:"layer_sizes"`
}

// ManifestStats holds statistics about the generated images.
type ManifestStats struct {
	TotalImages       int   `json:"total_images"`
	TotalSize         int64 `json:"total_size"`
	AverageSize       int64 `json:"average_size"`
	ImagesWithSharing int   `json:"images_with_sharing"`
	TotalLayers       int   `json:"total_layers"`
	AverageLayerSize  int64 `json:"average_layer_size"`
}

// Generator generates variant container images.
type Generator struct {
	config    Config
	logger    *zap.Logger
	generated []ImageEntry
	mu        sync.Mutex
}

// NewGenerator creates a new image generator.
func NewGenerator(config Config, logger *zap.Logger) *Generator {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &Generator{
		config:    config,
		logger:    logger,
		generated: make([]ImageEntry, 0, config.NumVariants),
	}
}

// Generate generates all variant images and returns the manifest.
func (g *Generator) Generate() (*ImageManifest, error) {
	startTime := time.Now()

	g.logger.Info("Starting image generation",
		zap.String("base_image", g.config.BaseImage),
		zap.Int("num_variants", g.config.NumVariants),
		zap.String("target_registry", g.config.TargetRegistry),
	)

	// Pull base image
	baseRef, err := name.ParseReference(g.config.BaseImage)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base image reference: %w", err)
	}

	baseImg, err := remote.Image(baseRef, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("failed to pull base image: %w", err)
	}

	g.logger.Info("Pulled base image", zap.String("ref", baseRef.String()))

	// Create work channel and result channel
	workChan := make(chan int, g.config.NumVariants)
	resultChan := make(chan error, g.config.NumVariants)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < g.config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for idx := range workChan {
				err := g.generateImage(idx, baseImg)
				resultChan <- err
			}
		}(i)
	}

	// Queue work
	for i := 0; i < g.config.NumVariants; i++ {
		workChan <- i
	}
	close(workChan)

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	successCount := 0
	errorCount := 0
	for err := range resultChan {
		if err != nil {
			g.logger.Error("Failed to generate image", zap.Error(err))
			errorCount++
		} else {
			successCount++
		}
	}

	if errorCount > 0 {
		g.logger.Warn("Some images failed to generate",
			zap.Int("success", successCount),
			zap.Int("errors", errorCount),
		)
	}

	// Build manifest
	manifest := g.buildManifest(startTime)

	g.logger.Info("Image generation complete",
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("total_images", manifest.Statistics.TotalImages),
		zap.Int64("total_size", manifest.Statistics.TotalSize),
	)

	return manifest, nil
}

func (g *Generator) generateImage(idx int, baseImg v1.Image) error {
	imageName := fmt.Sprintf("e2e-test-%03d", idx)
	tag := "v1"

	g.logger.Debug("Generating image",
		zap.Int("index", idx),
		zap.String("name", imageName),
	)

	// Determine if this image should be based on a previously generated image
	var parentRef string
	img := baseImg

	if idx > 0 && g.shouldShareLayers() {
		parentEntry := g.getRandomPreviousImage()
		if parentEntry != nil {
			parentRef = parentEntry.Reference
			ref, err := name.ParseReference(parentRef)
			if err == nil {
				opts := g.remoteOptions()
				if parentImg, err := remote.Image(ref, opts...); err == nil {
					img = parentImg
					g.logger.Debug("Using parent image",
						zap.String("parent", parentRef),
						zap.String("child", imageName),
					)
				}
			}
		}
	}

	// Determine number of layers to add
	numLayers := g.randomInt(g.config.MinLayers, g.config.MaxLayers)

	// Generate random layers
	var layerSizes []int64
	var totalSize int64

	currentImg := img

	for i := 0; i < numLayers; i++ {
		size := WeightedRandomSize()
		layerSizes = append(layerSizes, size)
		totalSize += size

		layer, err := RandomLayer(size)
		if err != nil {
			return fmt.Errorf("failed to create layer %d: %w", i, err)
		}

		currentImg, err = mutate.AppendLayers(currentImg, layer)
		if err != nil {
			return fmt.Errorf("failed to append layer %d: %w", i, err)
		}
	}

	// Push to registry
	targetRef, err := name.ParseReference(
		fmt.Sprintf("%s/%s:%s", g.config.TargetRegistry, imageName, tag),
		name.Insecure,
	)
	if err != nil {
		return fmt.Errorf("failed to parse target reference: %w", err)
	}

	opts := g.remoteOptions()

	if err := remote.Write(targetRef, currentImg, opts...); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	// Get digest
	digest, err := currentImg.Digest()
	if err != nil {
		return fmt.Errorf("failed to get digest: %w", err)
	}

	// Record the generated image
	entry := ImageEntry{
		Name:       imageName,
		Tag:        tag,
		Reference:  targetRef.String(),
		Digest:     digest.String(),
		NumLayers:  numLayers,
		TotalSize:  totalSize,
		ParentRef:  parentRef,
		LayerSizes: layerSizes,
	}

	g.mu.Lock()
	g.generated = append(g.generated, entry)
	g.mu.Unlock()

	g.logger.Info("Generated image",
		zap.String("ref", targetRef.String()),
		zap.Int("layers", numLayers),
		zap.Int64("size", totalSize),
	)

	return nil
}

func (g *Generator) remoteOptions() []remote.Option {
	opts := []remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}
	if g.config.Insecure {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
		}
		opts = append(opts, remote.WithTransport(transport))
	}
	return opts
}

func (g *Generator) shouldShareLayers() bool {
	// Generate random byte
	randByte := make([]byte, 1)
	rand.Read(randByte) //nolint:errcheck

	// Convert to percentage (0-100)
	pct := int(randByte[0]) * 100 / 256
	return pct < g.config.LayerSharingPercent
}

func (g *Generator) getRandomPreviousImage() *ImageEntry {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.generated) == 0 {
		return nil
	}

	// Get random index
	randBytes := make([]byte, 4)
	rand.Read(randBytes) //nolint:errcheck
	idx := int(randBytes[0]) % len(g.generated)

	return &g.generated[idx]
}

func (g *Generator) randomInt(min, max int) int {
	if min >= max {
		return min
	}

	randBytes := make([]byte, 4)
	rand.Read(randBytes) //nolint:errcheck

	rangeSize := max - min + 1
	val := int(randBytes[0]) | int(randBytes[1])<<8 | int(randBytes[2])<<16 | int(randBytes[3])<<24
	if val < 0 {
		val = -val
	}

	return min + (val % rangeSize)
}

func (g *Generator) buildManifest(startTime time.Time) *ImageManifest {
	g.mu.Lock()
	defer g.mu.Unlock()

	var totalSize int64
	var totalLayers int
	sharingCount := 0

	for _, img := range g.generated {
		totalSize += img.TotalSize
		totalLayers += img.NumLayers
		if img.ParentRef != "" {
			sharingCount++
		}
	}

	numImages := len(g.generated)
	var avgSize, avgLayerSize int64
	if numImages > 0 {
		avgSize = totalSize / int64(numImages)
	}
	if totalLayers > 0 {
		avgLayerSize = totalSize / int64(totalLayers)
	}

	return &ImageManifest{
		GeneratedAt: startTime,
		BaseImage:   g.config.BaseImage,
		Images:      g.generated,
		Statistics: ManifestStats{
			TotalImages:       numImages,
			TotalSize:         totalSize,
			AverageSize:       avgSize,
			ImagesWithSharing: sharingCount,
			TotalLayers:       totalLayers,
			AverageLayerSize:  avgLayerSize,
		},
	}
}

// WriteManifest writes the manifest to a file.
func (m *ImageManifest) WriteManifest(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// LoadManifest loads a manifest from a file.
func LoadManifest(path string) (*ImageManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ImageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &manifest, nil
}
