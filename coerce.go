package gnomon

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// toFloat converts the assorted numeric types pgx may return into a float64.
func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case nil:
		return 0, fmt.Errorf("gnomon: value is NULL")
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case pgtype.Numeric:
		f, err := n.Float64Value()
		if err != nil {
			return 0, err
		}
		if !f.Valid {
			return 0, fmt.Errorf("gnomon: numeric value is NULL")
		}
		return f.Float64, nil
	default:
		return 0, fmt.Errorf("gnomon: cannot convert %T to float64", v)
	}
}
