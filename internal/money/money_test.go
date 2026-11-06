package money

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		amount   int64
		currency string
		decimals int
		want     string
	}{
		{0, "so'm", 0, "0 so'm"},
		{150000, "so'm", 0, "150 000 so'm"},
		{999, "so'm", 0, "999 so'm"},
		{1000, "", 0, "1 000"},
		{-1234567, "so'm", 0, "-1 234 567 so'm"},
		{1250, "USD", 2, "12,50 USD"},
		{5, "USD", 2, "0,05 USD"},
		{100000000000, "so'm", 0, "100 000 000 000 so'm"},
		{-9223372036854775808, "", 0, "-9 223 372 036 854 775 808"},
	}
	for _, tt := range tests {
		if got := Format(tt.amount, tt.currency, tt.decimals); got != tt.want {
			t.Errorf("Format(%d,%q,%d) = %q, want %q", tt.amount, tt.currency, tt.decimals, got, tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in       string
		decimals int
		want     int64
		wantErr  string
	}{
		{"150000", 0, 150000, ""},
		{" 150 000 ", 0, 150000, ""},
		{"150 000", 0, 150000, ""},
		{"0", 0, 0, ""},
		{"", 0, 0, "Enter an amount."},
		{"   ", 0, 0, "Enter an amount."},
		{"150.5", 0, 0, "Enter whole so'm without decimals, e.g. 150000"},
		{"150,5", 0, 0, "Enter whole so'm without decimals, e.g. 150000"},
		{"-5", 0, 0, "Enter whole so'm without decimals, e.g. 150000"},
		{"abc", 0, 0, "Enter whole so'm without decimals, e.g. 150000"},
		{"99999999999999999999", 0, 0, "Amount is too large."},
		{"12.50", 2, 1250, ""},
		{"12,5", 2, 1250, ""},
		{"12", 2, 1200, ""},
		{".5", 2, 0, "Amount must be a plain number."},
		{"12.", 2, 1200, ""},
		{"1.2.3", 2, 0, "Amount has too many decimal points."},
		{"12.345", 2, 0, "Amount allows at most 2 decimal places."},
		{"1.23", 1, 0, "Amount allows at most 1 decimal place."},
		{"1x", 2, 0, "Amount must be a plain number."},
		{"9223372036854775807", 2, 0, "Amount is too large."},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, tt.decimals)
		switch {
		case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
			t.Errorf("Parse(%q,%d) error = %v, want %q", tt.in, tt.decimals, err, tt.wantErr)
		case tt.wantErr == "" && err != nil:
			t.Errorf("Parse(%q,%d) unexpected error %v", tt.in, tt.decimals, err)
		case tt.wantErr == "" && got != tt.want:
			t.Errorf("Parse(%q,%d) = %d, want %d", tt.in, tt.decimals, got, tt.want)
		}
	}
}

func TestParseWhole(t *testing.T) {
	if n, err := ParseWhole(" 42 ", false, ""); err != nil || n != 42 {
		t.Errorf("got %d %v", n, err)
	}
	if n, err := ParseWhole("-3", true, ""); err != nil || n != -3 {
		t.Errorf("got %d %v", n, err)
	}
	for _, bad := range []string{"", "-3", "1.5", "1 000", "x", "99999999999999999999"} {
		if _, err := ParseWhole(bad, false, "Stock must be a whole number."); err == nil || err.Error() != "Stock must be a whole number." {
			t.Errorf("ParseWhole(%q) error = %v", bad, err)
		}
	}
	if _, err := ParseWhole("-", true, ""); err == nil || err.Error() != "Enter a whole number." {
		t.Errorf("lone minus: %v", err)
	}
}

func TestFormatInputRoundTrips(t *testing.T) {
	for _, decimals := range []int{0, 1, 2, 3} {
		for _, amount := range []int64{0, 5, 999, 1250, 150000, 100000000000} {
			got, err := Parse(FormatInput(amount, decimals), decimals)
			if err != nil || got != amount {
				t.Errorf("round trip %d (decimals %d): %q -> %d, %v", amount, decimals, FormatInput(amount, decimals), got, err)
			}
		}
	}
}
