package action

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

func canonicalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		normalized, err := normalizeJSONNumber(string(typed))
		if err != nil {
			return nil, err
		}
		return json.RawMessage(normalized), nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			canonical, err := canonicalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[key] = canonical
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			canonical, err := canonicalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[index] = canonical
		}
		return result, nil
	default:
		return value, nil
	}
}

func normalizeJSONNumber(value string) (string, error) {
	if len(value) > MaxJSONNumberBytes {
		return "", fmt.Errorf("JSON number exceeds %d bytes", MaxJSONNumberBytes)
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	mantissa := value
	exponent := new(big.Int)
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa = value[:index]
		if _, ok := exponent.SetString(value[index+1:], 10); !ok {
			return "", fmt.Errorf("invalid JSON number exponent")
		}
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	coefficient := strings.TrimLeft(mantissa, "0")
	if coefficient == "" {
		return "0", nil
	}
	exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
	trimmed := strings.TrimRight(coefficient, "0")
	exponent.Add(exponent, big.NewInt(int64(len(coefficient)-len(trimmed))))
	coefficient = trimmed
	if negative {
		coefficient = "-" + coefficient
	}
	if exponent.Sign() == 0 {
		return coefficient, nil
	}
	return renderCanonicalNumber(coefficient, exponent), nil
}

func renderCanonicalNumber(coefficient string, exponent *big.Int) string {
	scientific := coefficient + "e" + exponent.String()
	negative := strings.HasPrefix(coefficient, "-")
	digits := coefficient
	if negative {
		digits = coefficient[1:]
	}
	signBytes := 0
	if negative {
		signBytes = 1
	}
	if exponent.Sign() > 0 && exponent.IsInt64() {
		zeros := exponent.Int64()
		if zeros <= int64(MaxJSONNumberBytes-len(coefficient)) {
			return coefficient + strings.Repeat("0", int(zeros))
		}
		return scientific
	}
	if exponent.Sign() >= 0 {
		return scientific
	}
	places := new(big.Int).Neg(new(big.Int).Set(exponent))
	if !places.IsInt64() {
		return scientific
	}
	fractionPlaces := places.Int64()
	if fractionPlaces < int64(len(digits)) {
		point := len(digits) - int(fractionPlaces)
		if len(coefficient)+1 <= MaxJSONNumberBytes {
			prefix := ""
			if negative {
				prefix = "-"
			}
			return prefix + digits[:point] + "." + digits[point:]
		}
		return scientific
	}
	leadingZeros := fractionPlaces - int64(len(digits))
	outputBytes := int64(signBytes+2+len(digits)) + leadingZeros
	if outputBytes <= MaxJSONNumberBytes {
		prefix := "0."
		if negative {
			prefix = "-0."
		}
		return prefix + strings.Repeat("0", int(leadingZeros)) + digits
	}
	return scientific
}
