package transform

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/cuducos/minha-receita/company"
)

const (
	dateInputFormat  = "20060102"
	dateOutputFormat = "2006-01-02"
)

func toInt(v string) (*int, error) {
	if v == "" {
		return nil, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return nil, fmt.Errorf("error converting %s to int: %w", v, err)
	}
	return &i, nil
}

func toFloat(v string) (*float32, error) {
	if v == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 32)
	if err != nil {
		return nil, fmt.Errorf("error converting %s to float32: %w", v, err)
	}
	f32 := float32(f)
	return &f32, nil
}

func toBool(v string) *bool {
	v = strings.ToUpper(v)
	var b bool
	switch v {
	case "S":
		b = true
	case "N":
		b = false
	default:
		return nil
	}
	return &b
}

// toDate expects a date as string in the format YYYYMMDD (that is the format
// used by the Federal Revenue in their CSV files).
func toDate(v string) (*company.Date, error) {
	onlyZeros := func(s string) bool {
		v, err := strconv.Atoi(s)
		if err != nil {
			return false
		}
		return v == 0
	}
	if v == "" || onlyZeros(v) {
		return nil, nil
	}
	t, err := time.Parse(dateInputFormat, v)
	if err != nil {
		return nil, fmt.Errorf("error converting %s to Time: %w", v, err)
	}
	d := company.Date(t)
	return &d, nil
}
