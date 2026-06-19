package gnomon

import (
	"fmt"
	"strconv"
)

// coerce converts a raw database value (as scanned into an any by pgx) into a
// float64 suitable for use as a Sample.Value. Supported types are the numeric
// scalars pgx produces when scanning into any: int, int32, int64, float32,
// float64, and string (parsed as a decimal number). nil is treated as zero.
// Any other type returns an error.
func coerce(v any) (float64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case int:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case float32:
		return float64(x), nil
	case float64:
		return x, nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, fmt.Errorf("gnomon: coerce: cannot parse %q as float64: %w", x, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("gnomon: coerce: unsupported type %T", v)
	}
}
