package api

import "testing"

func TestValidatePriceAcceptsUpperRange(t *testing.T) {
	if err := validatePrice(50_000_000_000); err != nil {
		t.Fatalf("validatePrice(50_000_000_000) = %v, want nil", err)
	}
	if err := validatePrice(100_000_000_000); err != nil {
		t.Fatalf("validatePrice(100_000_000_000) = %v, want nil", err)
	}
}

func TestValidatePriceRejectsAboveMax(t *testing.T) {
	if err := validatePrice(100_000_000_001); err == nil {
		t.Fatal("validatePrice(100_000_000_001) = nil, want error")
	}
}

func TestValidateBilliardAmountAcceptsUpperRange(t *testing.T) {
	if err := validateBilliardAmount(50_000_000_000); err != nil {
		t.Fatalf("validateBilliardAmount(50_000_000_000) = %v, want nil", err)
	}
}

func TestValidateBilliardAmountRejectsAboveMax(t *testing.T) {
	if err := validateBilliardAmount(100_000_000_001); err == nil {
		t.Fatal("validateBilliardAmount(100_000_000_001) = nil, want error")
	}
}
