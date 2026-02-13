package data

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	types2 "github.com/google/go-containerregistry/pkg/v1/types"
)

// IndexManifestResponse contains an image index manifest and its metadata.
type IndexManifestResponse struct {
	// Manifest is the parsed index manifest.
	Manifest *v1.IndexManifest
	// RawManifest is the raw manifest bytes for serialization.
	RawManifest []byte
	// Digest is the content-addressable hash of the manifest.
	Digest v1.Hash
	// Size is the size of the manifest in bytes.
	Size int64
	// MediaType is the media type of the manifest.
	MediaType types2.MediaType
}

// ImageManifestResponse contains a single image manifest and its metadata.
type ImageManifestResponse struct {
	// Manifest is the parsed image manifest.
	Manifest *v1.Manifest
	// RawManifest is the raw manifest bytes for serialization.
	RawManifest []byte
	// Digest is the content-addressable hash of the manifest.
	Digest v1.Hash
	// Size is the size of the manifest in bytes.
	Size int64
	// MediaType is the media type of the manifest.
	MediaType types2.MediaType
}
