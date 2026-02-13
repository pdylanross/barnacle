package coordinator

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSpaceReserver_ReserveAndClaimedBytes(t *testing.T) {
	sr := NewSpaceReserver(5*time.Minute, zap.NewNop())

	r1 := sr.Reserve(0, 100)
	r2 := sr.Reserve(0, 200)

	assert.Equal(t, uint64(300), sr.ClaimedBytes(0))
	assert.NotNil(t, r1)
	assert.NotNil(t, r2)
}

func TestSpaceReserver_ReleaseReducesClaimed(t *testing.T) {
	sr := NewSpaceReserver(5*time.Minute, zap.NewNop())

	r1 := sr.Reserve(0, 100)
	r2 := sr.Reserve(0, 200)

	r1.Release()
	assert.Equal(t, uint64(200), sr.ClaimedBytes(0))

	r2.Release()
	assert.Equal(t, uint64(0), sr.ClaimedBytes(0))
}

func TestSpaceReserver_ReleaseIdempotent(t *testing.T) {
	sr := NewSpaceReserver(5*time.Minute, zap.NewNop())

	r := sr.Reserve(0, 500)
	assert.Equal(t, uint64(500), sr.ClaimedBytes(0))

	r.Release()
	r.Release() // double release
	r.Release() // triple release

	assert.Equal(t, uint64(0), sr.ClaimedBytes(0))
}

func TestSpaceReserver_NilReservationRelease(t *testing.T) {
	// Ensure calling Release on a nil *Reservation does not panic.
	var r *Reservation
	require.NotPanics(t, func() {
		r.Release()
	})
}

func TestSpaceReserver_MultipleTiersIndependent(t *testing.T) {
	sr := NewSpaceReserver(5*time.Minute, zap.NewNop())

	sr.Reserve(0, 100)
	sr.Reserve(1, 200)
	sr.Reserve(0, 50)
	sr.Reserve(2, 300)

	assert.Equal(t, uint64(150), sr.ClaimedBytes(0))
	assert.Equal(t, uint64(200), sr.ClaimedBytes(1))
	assert.Equal(t, uint64(300), sr.ClaimedBytes(2))
	assert.Equal(t, uint64(0), sr.ClaimedBytes(99)) // non-existent tier
}

func TestSpaceReserver_TTLExpiry(t *testing.T) {
	sr := NewSpaceReserver(50*time.Millisecond, zap.NewNop())

	sr.Reserve(0, 500)
	assert.Equal(t, uint64(500), sr.ClaimedBytes(0))

	time.Sleep(100 * time.Millisecond)

	// After TTL, claimed bytes should be 0 due to lazy pruning
	assert.Equal(t, uint64(0), sr.ClaimedBytes(0))
}

func TestSpaceReserver_ConcurrentAccess(t *testing.T) {
	sr := NewSpaceReserver(5*time.Minute, zap.NewNop())

	const goroutines = 100
	var wg sync.WaitGroup
	reservations := make([]*Reservation, goroutines)

	// Concurrently reserve
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			reservations[idx] = sr.Reserve(idx%3, 10)
		}(i)
	}
	wg.Wait()

	// Verify total
	total := sr.ClaimedBytes(0) + sr.ClaimedBytes(1) + sr.ClaimedBytes(2)
	assert.Equal(t, uint64(goroutines*10), total)

	// Concurrently release
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			reservations[idx].Release()
		}(i)
	}
	wg.Wait()

	assert.Equal(t, uint64(0), sr.ClaimedBytes(0))
	assert.Equal(t, uint64(0), sr.ClaimedBytes(1))
	assert.Equal(t, uint64(0), sr.ClaimedBytes(2))
}
