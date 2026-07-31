package csapi

import (
	"strings"
	"testing"
)

func TestDefaultConfigBoundsObservationStreamStorage(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if got, want := cfg.ObservationsMaxBytes, int64(1<<30); got != want {
		t.Fatalf("ObservationsMaxBytes = %d, want %d", got, want)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() = %v", err)
	}
}

func TestConfigValidateRequiresPositiveObservationStreamLimit(t *testing.T) {
	t.Parallel()

	for _, maxBytes := range []int64{0, -1} {
		cfg := DefaultConfig()
		cfg.ObservationsMaxBytes = maxBytes
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "observations_max_bytes must be > 0") {
			t.Errorf("ObservationsMaxBytes=%d: Validate() = %v, want positive-limit error", maxBytes, err)
		}
	}
}

func TestApplyDefaultsDoesNotMaskExplicitZeroObservationStreamLimit(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ObservationsMaxBytes = 0
	cfg.ApplyDefaults()
	if cfg.ObservationsMaxBytes != 0 {
		t.Fatalf("ApplyDefaults changed explicit zero MaxBytes to %d", cfg.ObservationsMaxBytes)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "observations_max_bytes must be > 0") {
		t.Fatalf("Validate() = %v, want explicit-zero rejection", err)
	}
}
