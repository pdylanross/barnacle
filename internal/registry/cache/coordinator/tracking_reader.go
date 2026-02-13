package coordinator

import (
	"io"
)

// ReleaseFunc is a function that is called when a tracked reader is closed.
type ReleaseFunc func()

// trackingReader wraps an [io.ReadCloser] and calls a release function on Close.
// This is used to track in-flight requests for rebalancing.
type trackingReader struct {
	reader  io.ReadCloser
	release ReleaseFunc
	closed  bool
}

// newTrackingReader creates a new tracking reader that calls release on Close.
// If release is nil, no tracking is performed.
func newTrackingReader(reader io.ReadCloser, release ReleaseFunc) io.ReadCloser {
	if release == nil {
		return reader
	}
	return &trackingReader{
		reader:  reader,
		release: release,
	}
}

// Read implements [io.Reader].
func (t *trackingReader) Read(p []byte) (int, error) {
	return t.reader.Read(p)
}

// Close closes the underlying reader and calls the release function.
// It is safe to call Close multiple times.
func (t *trackingReader) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true

	// Call release first to decrement the counter
	if t.release != nil {
		t.release()
	}

	// Then close the underlying reader
	return t.reader.Close()
}
