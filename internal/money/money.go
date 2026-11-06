// Package money formats and parses amounts the same way the web pages do (web/static/app.js),
// with integer maths only: amounts are whole minor units (whole so'm when decimals is 0).
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// NBSP separates thousands groups and the currency label, so "150 000 so'm" never wraps.
const NBSP = " "

// Format turns an amount in minor units into a display string like "150 000 so'm".
func Format(amount int64, currency string, decimals int) string {
	negative := amount < 0
	abs := uint64(amount)
	if negative {
		abs = -uint64(amount) // also right for math.MinInt64
	}

	var scale uint64 = 1
	for i := 0; i < decimals; i++ {
		scale *= 10
	}
	whole, frac := abs/scale, abs%scale

	display := groupThousands(strconv.FormatUint(whole, 10))
	if decimals > 0 {
		display += "," + fmt.Sprintf("%0*d", decimals, frac)
	}
	if negative {
		display = "-" + display
	}
	if currency != "" {
		display += NBSP + currency
	}
	return display
}

// groupThousands inserts NBSP every 3 digits from the right: "150000" -> "150 000".
func groupThousands(digits string) string {
	var b strings.Builder
	for i := 0; i < len(digits); i++ {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteString(NBSP)
		}
		b.WriteByte(digits[i])
	}
	return b.String()
}

// Parse turns user input into an amount in minor units. With decimals 0 it accepts only digits
// and spaces (as thousands separators); "." and "," are refused. With decimals > 0 it accepts
// one "," or "." and at most that many decimals. The error text is meant for the user.
func Parse(input string, decimals int) (int64, error) {
	s := strings.Join(strings.Fields(strings.ReplaceAll(input, NBSP, " ")), "")
	if s == "" {
		return 0, errors.New("Enter an amount.")
	}

	if decimals == 0 {
		if !allDigits(s) {
			return 0, errors.New("Enter whole so'm without decimals, e.g. 150000")
		}
		return atoi(s)
	}

	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, errors.New("Amount has too many decimal points.")
	}
	whole, frac := parts[0], ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if !allDigits(whole) || (frac != "" && !allDigits(frac)) {
		return 0, errors.New("Amount must be a plain number.")
	}
	if len(frac) > decimals {
		plural := "s"
		if decimals == 1 {
			plural = ""
		}
		return 0, fmt.Errorf("Amount allows at most %d decimal place%s.", decimals, plural)
	}
	frac += strings.Repeat("0", decimals-len(frac))

	w, err := atoi(whole)
	if err != nil {
		return 0, err
	}
	var f int64
	if frac != "" {
		if f, err = atoi(frac); err != nil {
			return 0, err
		}
	}
	var scale int64 = 1
	for i := 0; i < decimals; i++ {
		scale *= 10
	}
	if w > (1<<63-1-f)/scale {
		return 0, errors.New("Amount is too large.")
	}
	return w*scale + f, nil
}

// ParseWhole reads a plain integer (stock, threshold, adjustment). message is the error text
// for bad input; empty means "Enter a whole number.".
func ParseWhole(input string, allowNegative bool, message string) (int64, error) {
	if message == "" {
		message = "Enter a whole number."
	}
	s := strings.TrimSpace(input)
	digits := s
	if allowNegative {
		digits = strings.TrimPrefix(s, "-")
	}
	if !allDigits(digits) {
		return 0, errors.New(message)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New(message)
	}
	return n, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New("Amount is too large.")
	}
	return n, nil
}

// FormatInput renders an amount for an edit box: no currency and no thousands grouping (the web
// form keeps the grouping; Parse accepts both). Whatever it returns, Parse reads back unchanged.
func FormatInput(amount int64, decimals int) string {
	if decimals == 0 {
		return strconv.FormatInt(amount, 10)
	}
	s := Format(amount, "", decimals)
	return strings.ReplaceAll(s, NBSP, "")
}
