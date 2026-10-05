package config

import "testing"

func TestLoadDefaultsToUzbekSom(t *testing.T) {
	t.Setenv("CURRENCY", "")
	t.Setenv("DECIMALS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Currency != "so'm" {
		t.Fatalf("currency = %q, want so'm", cfg.Currency)
	}
	if cfg.Decimals != 0 {
		t.Fatalf("decimals = %d, want 0", cfg.Decimals)
	}
}

func TestLoadCurrencyAndDecimalsOverridable(t *testing.T) {
	t.Setenv("CURRENCY", "USD")
	t.Setenv("DECIMALS", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", cfg.Currency)
	}
	if cfg.Decimals != 2 {
		t.Fatalf("decimals = %d, want 2", cfg.Decimals)
	}
}

func TestLoadDecimalsOutOfRangeRejected(t *testing.T) {
	t.Setenv("DECIMALS", "4")

	if _, err := Load(); err == nil {
		t.Fatal("want error for DECIMALS=4, got nil")
	}
}
