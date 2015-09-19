package flocker

import "os"

// GetenvOrDefault returns the value of the enviroment variable if it's set,
// otherwise it return the default value provided.
//
// Note: it should be GetEnv but I tried to keep the os.Getenv standard.
func GetenvOrFallback(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
