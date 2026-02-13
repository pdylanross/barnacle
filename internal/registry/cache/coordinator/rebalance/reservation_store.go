package rebalance

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pdylanross/barnacle/internal/registry/cache/coordinator"
	"github.com/pdylanross/barnacle/pkg/configuration"
	"go.uber.org/zap"
)

// PendingReservation represents a reservation for incoming blob data.
type PendingReservation struct {
	// ID is the unique identifier for this reservation.
	ID string
	// Digest is the content-addressable digest of the blob.
	Digest string
	// Size is the expected size of the blob in bytes.
	Size int64
	// MediaType is the OCI media type of the blob.
	MediaType string
	// SourceNodeID is the node sending the blob.
	SourceNodeID string
	// Tier is the cache tier where the blob will be stored.
	Tier int
	// SpaceReservation is the underlying disk space reservation.
	SpaceReservation *coordinator.Reservation
	// CreatedAt is when this reservation was created.
	CreatedAt time.Time
	// ExpiresAt is when this reservation expires.
	ExpiresAt time.Time
}

// ReservationStore manages pending reservations for incoming blob transfers.
type ReservationStore struct {
	mu           sync.RWMutex
	reservations map[string]*PendingReservation
	config       *configuration.RebalanceConfiguration
	logger       *zap.Logger
}

// NewReservationStore creates a new ReservationStore.
func NewReservationStore(
	config *configuration.RebalanceConfiguration,
	logger *zap.Logger,
) *ReservationStore {
	store := &ReservationStore{
		reservations: make(map[string]*PendingReservation),
		config:       config,
		logger:       logger.Named("reservation-store"),
	}

	// Start background cleanup goroutine
	go store.cleanupLoop()

	return store
}

// Create creates a new reservation and returns it.
func (s *ReservationStore) Create(
	digest string,
	size int64,
	mediaType string,
	sourceNodeID string,
	tier int,
	spaceReservation *coordinator.Reservation,
) *PendingReservation {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ttl := s.config.GetReservationTTL()

	r := &PendingReservation{
		ID:               uuid.New().String(),
		Digest:           digest,
		Size:             size,
		MediaType:        mediaType,
		SourceNodeID:     sourceNodeID,
		Tier:             tier,
		SpaceReservation: spaceReservation,
		CreatedAt:        now,
		ExpiresAt:        now.Add(ttl),
	}

	s.reservations[r.ID] = r

	s.logger.Debug("created reservation",
		zap.String("reservationID", r.ID),
		zap.String("digest", digest),
		zap.Int64("size", size),
		zap.Time("expiresAt", r.ExpiresAt))

	return r
}

// Get retrieves a reservation by ID.
// Returns nil if not found or expired.
func (s *ReservationStore) Get(id string) *PendingReservation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.reservations[id]
	if !ok {
		return nil
	}

	if time.Now().After(r.ExpiresAt) {
		return nil
	}

	return r
}

// GetByDigest retrieves a reservation by digest.
// Returns nil if not found or expired.
func (s *ReservationStore) GetByDigest(digest string) *PendingReservation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.reservations {
		if r.Digest == digest && time.Now().Before(r.ExpiresAt) {
			return r
		}
	}

	return nil
}

// Complete marks a reservation as completed and removes it from the store.
// This should be called after successful blob storage.
func (s *ReservationStore) Complete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reservations[id]
	if !ok {
		return
	}

	// Release the underlying space reservation
	if r.SpaceReservation != nil {
		r.SpaceReservation.Release()
	}

	delete(s.reservations, id)

	s.logger.Debug("completed reservation",
		zap.String("reservationID", id),
		zap.String("digest", r.Digest))
}

// Cancel cancels a reservation and releases its resources.
func (s *ReservationStore) Cancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reservations[id]
	if !ok {
		return
	}

	// Release the underlying space reservation
	if r.SpaceReservation != nil {
		r.SpaceReservation.Release()
	}

	delete(s.reservations, id)

	s.logger.Debug("cancelled reservation",
		zap.String("reservationID", id),
		zap.String("digest", r.Digest))
}

// cleanupLoop periodically removes expired reservations.
func (s *ReservationStore) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second) //nolint:mnd // 30 seconds is a reasonable cleanup interval
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes expired reservations.
func (s *ReservationStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var expired []string

	for id, r := range s.reservations {
		if now.After(r.ExpiresAt) {
			expired = append(expired, id)

			// Release the underlying space reservation
			if r.SpaceReservation != nil {
				r.SpaceReservation.Release()
			}
		}
	}

	for _, id := range expired {
		delete(s.reservations, id)
	}

	if len(expired) > 0 {
		s.logger.Debug("cleaned up expired reservations",
			zap.Int("count", len(expired)))
	}
}

// Count returns the number of active reservations.
func (s *ReservationStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, r := range s.reservations {
		if now.Before(r.ExpiresAt) {
			count++
		}
	}

	return count
}
