package configuration

import (
	"fmt"
	"time"
)

// Default rebalance configuration values.
const (
	DefaultRebalanceCooldownDuration       = 1 * time.Minute
	DefaultRebalanceQueuePollInterval      = 1 * time.Second
	DefaultRebalanceTransferTimeout        = 10 * time.Minute
	DefaultRebalanceMaxConcurrentTransfers = 2
	DefaultRebalanceInFlightDrainTimeout   = 30 * time.Second
	DefaultRebalanceReservationTTL         = 5 * time.Minute
	DefaultRebalanceAccessWindow           = 24 * time.Hour
	DefaultRebalanceInterval               = 30 * time.Second
)

// RebalanceConfiguration contains settings for the blob rebalancing system.
type RebalanceConfiguration struct {
	// Enabled controls whether the rebalancing system is active.
	Enabled bool `koanf:"enabled"`

	// CooldownDuration is the minimum time between rebalancing the same blob.
	// Defaults to 1 minute if not specified.
	CooldownDuration time.Duration `koanf:"cooldownDuration"`

	// QueuePollInterval is how often workers poll their queue when empty.
	// Defaults to 1 second if not specified.
	QueuePollInterval time.Duration `koanf:"queuePollInterval"`

	// TransferTimeout is the maximum time allowed for a single blob transfer.
	// Defaults to 10 minutes if not specified.
	TransferTimeout time.Duration `koanf:"transferTimeout"`

	// MaxConcurrentTransfers limits how many transfers a node can execute at once.
	// Defaults to 2 if not specified.
	MaxConcurrentTransfers int `koanf:"maxConcurrentTransfers"`

	// InFlightDrainTimeout is how long to wait for in-flight requests to drain
	// before deleting a blob from the source node.
	// Defaults to 30 seconds if not specified.
	InFlightDrainTimeout time.Duration `koanf:"inFlightDrainTimeout"`

	// ReservationTTL is how long a disk space reservation is valid.
	// Defaults to 5 minutes if not specified.
	ReservationTTL time.Duration `koanf:"reservationTTL"`

	// AccessWindow is the sliding window duration for counting blob accesses
	// when scoring blobs for tier placement. Defaults to 24 hours if not specified.
	AccessWindow time.Duration `koanf:"accessWindow"`

	// RebalanceInterval is how often the leader runs a rebalance cycle.
	// The leader loop heartbeats every second to maintain the lock, but only
	// executes rebalancing at this interval. Defaults to 30 seconds if not specified.
	RebalanceInterval time.Duration `koanf:"rebalanceInterval"`
}

// GetCooldownDuration returns the cooldown duration, using the default if not set.
func (c *RebalanceConfiguration) GetCooldownDuration() time.Duration {
	if c.CooldownDuration == 0 {
		return DefaultRebalanceCooldownDuration
	}
	return c.CooldownDuration
}

// GetQueuePollInterval returns the queue poll interval, using the default if not set.
func (c *RebalanceConfiguration) GetQueuePollInterval() time.Duration {
	if c.QueuePollInterval == 0 {
		return DefaultRebalanceQueuePollInterval
	}
	return c.QueuePollInterval
}

// GetTransferTimeout returns the transfer timeout, using the default if not set.
func (c *RebalanceConfiguration) GetTransferTimeout() time.Duration {
	if c.TransferTimeout == 0 {
		return DefaultRebalanceTransferTimeout
	}
	return c.TransferTimeout
}

// GetMaxConcurrentTransfers returns the max concurrent transfers, using the default if not set.
func (c *RebalanceConfiguration) GetMaxConcurrentTransfers() int {
	if c.MaxConcurrentTransfers == 0 {
		return DefaultRebalanceMaxConcurrentTransfers
	}
	return c.MaxConcurrentTransfers
}

// GetInFlightDrainTimeout returns the in-flight drain timeout, using the default if not set.
func (c *RebalanceConfiguration) GetInFlightDrainTimeout() time.Duration {
	if c.InFlightDrainTimeout == 0 {
		return DefaultRebalanceInFlightDrainTimeout
	}
	return c.InFlightDrainTimeout
}

// GetReservationTTL returns the reservation TTL, using the default if not set.
func (c *RebalanceConfiguration) GetReservationTTL() time.Duration {
	if c.ReservationTTL == 0 {
		return DefaultRebalanceReservationTTL
	}
	return c.ReservationTTL
}

// GetAccessWindow returns the access window duration, using the default if not set.
func (c *RebalanceConfiguration) GetAccessWindow() time.Duration {
	if c.AccessWindow == 0 {
		return DefaultRebalanceAccessWindow
	}
	return c.AccessWindow
}

// GetRebalanceInterval returns the rebalance interval, using the default if not set.
func (c *RebalanceConfiguration) GetRebalanceInterval() time.Duration {
	if c.RebalanceInterval == 0 {
		return DefaultRebalanceInterval
	}
	return c.RebalanceInterval
}

// Validate checks that the rebalance configuration is valid.
func (c *RebalanceConfiguration) Validate() error {
	if c.CooldownDuration < 0 {
		return fmt.Errorf("%w: cooldownDuration must be non-negative", ErrInvalidConfiguration)
	}
	if c.QueuePollInterval < 0 {
		return fmt.Errorf("%w: queuePollInterval must be non-negative", ErrInvalidConfiguration)
	}
	if c.TransferTimeout < 0 {
		return fmt.Errorf("%w: transferTimeout must be non-negative", ErrInvalidConfiguration)
	}
	if c.MaxConcurrentTransfers < 0 {
		return fmt.Errorf("%w: maxConcurrentTransfers must be non-negative", ErrInvalidConfiguration)
	}
	if c.InFlightDrainTimeout < 0 {
		return fmt.Errorf("%w: inFlightDrainTimeout must be non-negative", ErrInvalidConfiguration)
	}
	if c.ReservationTTL < 0 {
		return fmt.Errorf("%w: reservationTTL must be non-negative", ErrInvalidConfiguration)
	}
	if c.AccessWindow < 0 {
		return fmt.Errorf("%w: accessWindow must be non-negative", ErrInvalidConfiguration)
	}
	if c.RebalanceInterval < 0 {
		return fmt.Errorf("%w: rebalanceInterval must be non-negative", ErrInvalidConfiguration)
	}
	return nil
}
