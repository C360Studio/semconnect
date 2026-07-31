package csapi

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestObservationStreamConfigIsBoundedAndDiscardsOldest(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	got := observationStreamConfig(cfg)

	if got.MaxBytes != cfg.ObservationsMaxBytes {
		t.Errorf("MaxBytes = %d, want %d", got.MaxBytes, cfg.ObservationsMaxBytes)
	}
	if got.MaxAge != 30*24*time.Hour {
		t.Errorf("MaxAge = %s, want 30 days", got.MaxAge)
	}
	if got.Discard != jetstream.DiscardOld {
		t.Errorf("Discard = %v, want DiscardOld", got.Discard)
	}
	if got.Retention != jetstream.LimitsPolicy {
		t.Errorf("Retention = %v, want LimitsPolicy", got.Retention)
	}
	if got.Storage != jetstream.FileStorage {
		t.Errorf("Storage = %v, want FileStorage", got.Storage)
	}
}
