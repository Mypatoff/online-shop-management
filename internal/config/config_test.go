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

func TestLoadLang(t *testing.T) {
	for _, tc := range []struct {
		name, shopLang, lang, want string
		wantErr                    bool
	}{
		{"default", "", "", "en", false},
		{"explicit", "uz", "", "uz", false},
		{"explicit beats LANG", "ru", "uz_UZ.UTF-8", "ru", false},
		{"LANG locale", "", "uz_UZ.UTF-8", "uz", false},
		{"LANG with dash", "", "ru-RU", "ru", false},
		{"LANG unsupported is ignored", "", "de_DE.UTF-8", "en", false},
		{"LANG C is ignored", "", "C", "en", false},
		{"typo in SHOP_LANG is an error", "uzbek", "", "", true},
		{"case does not matter", "RU", "", "ru", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHOP_LANG", tc.shopLang)
			t.Setenv("LANG", tc.lang)
			cfg, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && cfg.Lang != tc.want {
				t.Fatalf("Lang = %q, want %q", cfg.Lang, tc.want)
			}
		})
	}
}
