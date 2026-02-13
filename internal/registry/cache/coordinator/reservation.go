package coordinator

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

const defaultReservationTTL = 5 * time.Minute

// SpaceReserver tracks "claimed but not yet written" bytes per cache tier.
// It prevents TOCTOU races where multiple concurrent goroutines all see the
// same free space before any have written their blob.
type SpaceReserver struct {
	mu           sync.Mutex
	reservations []*Reservation
	ttl          time.Duration
	logger       *zap.Logger
}

// Reservation represents a claim on disk space in a specific cache tier.
// Callers must call Release when the blob write completes (success or failure).
type Reservation struct {
	tierIndex int
	size      uint64
	createdAt time.Time
	released  bool
	reserver  *SpaceReserver
}

// NewSpaceReserver creates a new SpaceReserver with the given TTL and logger.
// If ttl is zero, it defaults to 5 minutes.
func NewSpaceReserver(ttl time.Duration, logger *zap.Logger) *SpaceReserver {
	if ttl == 0 {
		ttl = defaultReservationTTL
	}
	return &SpaceReserver{
		ttl:    ttl,
		logger: logger.Named("spaceReserver"),
	}
}

// Reserve creates a reservation for the given number of bytes on the specified tier.
func (s *SpaceReserver) Reserve(tierIndex int, size uint64) *Reservation {
	r := &Reservation{
		tierIndex: tierIndex,
		size:      size,
		createdAt: time.Now(),
		reserver:  s,
	}

	s.mu.Lock()
	s.reservations = append(s.reservations, r)
	s.mu.Unlock()

	s.logger.Debug("space reserved",
		zap.Int("tierIndex", tierIndex),
		zap.Uint64("size", size))

	return r
}

// ClaimedBytes returns the total claimed bytes for the given tier index,
// lazily pruning expired reservations.
func (s *SpaceReserver) ClaimedBytes(tierIndex int) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var total uint64
	n := 0

	for _, r := range s.reservations {
		if r.released {
			continue
		}
		if now.Sub(r.createdAt) >= s.ttl {
			s.logger.Warn("reservation expired, pruning",
				zap.Int("tierIndex", r.tierIndex),
				zap.Uint64("size", r.size))
			r.released = true
			continue
		}
		// Keep this reservation
		s.reservations[n] = r
		n++
		if r.tierIndex == tierIndex {
			total += r.size
		}
	}

	// Clear trailing references for GC
	for i := n; i < len(s.reservations); i++ {
		s.reservations[i] = nil
	}
	s.reservations = s.reservations[:n]

	return total
}

// release marks a reservation as released and compacts the slice. It is idempotent.
func (s *SpaceReserver) release(r *Reservation) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.released {
		return
	}
	r.released = true

	s.logger.Debug("reservation released",
		zap.Int("tierIndex", r.tierIndex),
		zap.Uint64("size", r.size))

	// Compact: remove this reservation from the slice
	n := 0
	for _, res := range s.reservations {
		if res == r {
			continue
		}
		s.reservations[n] = res
		n++
	}
	for i := n; i < len(s.reservations); i++ {
		s.reservations[i] = nil
	}
	s.reservations = s.reservations[:n]
}

// Release releases this reservation's claimed space. It is safe to call multiple times.
func (r *Reservation) Release() {
	if r == nil {
		return
	}
	r.reserver.release(r)
}
