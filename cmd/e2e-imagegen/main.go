package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/pdylanross/barnacle/test/e2e/imagegen"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse command line flags
	var (
		baseImage      = flag.String("base", "alpine:latest", "Base image to use for generation")
		registry       = flag.String("registry", "localhost:5000", "Target registry to push images to")
		numVariants    = flag.Int("variants", 100, "Number of image variants to generate")
		minLayers      = flag.Int("min-layers", 1, "Minimum number of layers per image")
		maxLayers      = flag.Int("max-layers", 4, "Maximum number of layers per image")
		sharingPercent = flag.Int("sharing", 50, "Percentage of images to base on previously generated images")
		concurrency    = flag.Int("concurrency", 4, "Number of concurrent image generations")
		insecure       = flag.Bool("insecure", true, "Allow pushing to insecure registries")
		output         = flag.String("output", "e2e-images.json", "Output manifest file path")
		verbose        = flag.Bool("verbose", false, "Enable verbose logging")
	)

	flag.Usage = func() { //nolint:reassign // customizing CLI usage output
		fmt.Fprintf(os.Stderr, "e2e-imagegen - Generate variant container images for e2e testing\n\n")
		fmt.Fprintf(os.Stderr, "Usage: e2e-imagegen [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Generate 100 images to local registry\n")
		fmt.Fprintf(os.Stderr, "  e2e-imagegen -registry localhost:5000 -variants 100\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate images with custom layer settings\n")
		fmt.Fprintf(os.Stderr, "  e2e-imagegen -min-layers 2 -max-layers 5 -variants 50\n\n")
	}

	flag.Parse()

	// Setup logger
	var logConfig zap.Config
	if *verbose {
		logConfig = zap.NewDevelopmentConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	} else {
		logConfig = zap.NewProductionConfig()
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	logger, err := logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck // best-effort sync on exit

	// Build configuration
	config := imagegen.Config{
		BaseImage:           *baseImage,
		TargetRegistry:      *registry,
		NumVariants:         *numVariants,
		MinLayers:           *minLayers,
		MaxLayers:           *maxLayers,
		LayerSharingPercent: *sharingPercent,
		Concurrency:         *concurrency,
		Insecure:            *insecure,
	}

	// Validate configuration
	if validateErr := validateConfig(config); validateErr != nil {
		logger.Fatal("Invalid configuration", zap.Error(validateErr))
	}

	logger.Info("Starting image generation",
		zap.String("base_image", config.BaseImage),
		zap.String("registry", config.TargetRegistry),
		zap.Int("variants", config.NumVariants),
		zap.Int("min_layers", config.MinLayers),
		zap.Int("max_layers", config.MaxLayers),
		zap.Int("sharing_percent", config.LayerSharingPercent),
		zap.Int("concurrency", config.Concurrency),
	)

	// Create generator and run
	generator := imagegen.NewGenerator(config, logger)
	manifest, err := generator.Generate()
	if err != nil {
		logger.Fatal("Image generation failed", zap.Error(err))
	}

	// Write manifest
	if writeErr := manifest.WriteManifest(*output); writeErr != nil {
		logger.Fatal("Failed to write manifest", zap.Error(writeErr))
	}

	logger.Info("Image generation complete",
		zap.String("manifest", *output),
		zap.Int("total_images", manifest.Statistics.TotalImages),
		zap.Int64("total_size_bytes", manifest.Statistics.TotalSize),
		zap.Int("images_with_sharing", manifest.Statistics.ImagesWithSharing),
	)

	// Print summary
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "=== Generation Summary ===")
	fmt.Fprintf(os.Stdout, "Total images:        %d\n", manifest.Statistics.TotalImages)
	fmt.Fprintf(os.Stdout, "Total size:          %s\n", formatBytes(manifest.Statistics.TotalSize))
	fmt.Fprintf(os.Stdout, "Average image size:  %s\n", formatBytes(manifest.Statistics.AverageSize))
	fmt.Fprintf(os.Stdout, "Total layers:        %d\n", manifest.Statistics.TotalLayers)
	fmt.Fprintf(os.Stdout, "Average layer size:  %s\n", formatBytes(manifest.Statistics.AverageLayerSize))
	fmt.Fprintf(os.Stdout, "Images with sharing: %d\n", manifest.Statistics.ImagesWithSharing)
	fmt.Fprintf(os.Stdout, "Manifest written to: %s\n", *output)
}

func validateConfig(config imagegen.Config) error {
	if config.NumVariants < 1 {
		return errors.New("variants must be at least 1")
	}
	if config.MinLayers < 1 {
		return errors.New("min-layers must be at least 1")
	}
	if config.MaxLayers < config.MinLayers {
		return errors.New("max-layers must be >= min-layers")
	}
	if config.LayerSharingPercent < 0 || config.LayerSharingPercent > 100 {
		return errors.New("sharing must be between 0 and 100")
	}
	if config.Concurrency < 1 {
		return errors.New("concurrency must be at least 1")
	}
	if config.BaseImage == "" {
		return errors.New("base image cannot be empty")
	}
	if config.TargetRegistry == "" {
		return errors.New("registry cannot be empty")
	}
	return nil
}

func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
