package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const MaxArrivalBody = 16 * 1024

// DecodeArrivalJSON bounds memory and rejects ambiguous or unknown request fields.
func DecodeArrivalJSON(r io.Reader, out any) error {
	data, err := io.ReadAll(io.LimitReader(r, MaxArrivalBody+1))
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if len(data) > MaxArrivalBody {
		return fmt.Errorf("request exceeds %d bytes", MaxArrivalBody)
	}
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return fmt.Errorf("request must be an object")
	}
	scan := json.NewDecoder(bytes.NewReader(data))
	scan.UseNumber()
	if err := scanValue(scan); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func scanValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid object key")
			}
			// encoding/json matches struct fields case-insensitively; reject
			// aliases as duplicates before that matching can overwrite a field.
			canonical := strings.ToLower(key)
			if seen[canonical] {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[canonical] = true
			if err := scanValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanValue(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}

// ParseVersion accepts only unsigned decimal strings, without rounding.
func ParseVersion(value string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("version is required")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("version must be an unsigned decimal string")
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("version overflows uint64: %w", err)
	}
	return n, nil
}

func ValidateIdempotencyKey(key string) error {
	if len(key) < 1 || len(key) > 128 {
		return fmt.Errorf("Idempotency-Key must contain 1–128 ASCII characters")
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return fmt.Errorf("Idempotency-Key must contain printable non-space ASCII characters")
		}
	}
	return nil
}
