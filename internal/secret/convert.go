package secret

import "fmt"

func stringifySecret(v interface{}) ([]byte, bool) {
	switch value := v.(type) {
	case string:
		return cloneBytes([]byte(value)), true
	case []byte:
		return cloneBytes(value), true
	case fmt.Stringer:
		return cloneBytes([]byte(value.String())), true
	default:
		return nil, false
	}
}
