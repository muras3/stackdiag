package cli

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// ResolveAuth resolves authentication from environment variables and sets
// the Authorization header on cfg. It returns an error instead of calling
// os.Exit, making it testable. The lookupEnv function should behave like
// os.LookupEnv.
func ResolveAuth(cfg *Config, lookupEnv func(string) (string, bool)) error {
	if cfg.BearerEnv != "" {
		val, ok := lookupEnv(cfg.BearerEnv)
		if !ok {
			return fmt.Errorf("environment variable %q is not set", cfg.BearerEnv)
		}
		if val == "" {
			return fmt.Errorf("environment variable %q is empty", cfg.BearerEnv)
		}
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("environment variable %q contains only whitespace", cfg.BearerEnv)
		}
		cfg.Headers["Authorization"] = "Bearer " + val
	}

	if cfg.BasicEnv != "" {
		val, ok := lookupEnv(cfg.BasicEnv)
		if !ok {
			return fmt.Errorf("environment variable %q is not set", cfg.BasicEnv)
		}
		if val == "" {
			return fmt.Errorf("environment variable %q is empty", cfg.BasicEnv)
		}
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("environment variable %q contains only whitespace", cfg.BasicEnv)
		}
		if !strings.Contains(val, ":") {
			return fmt.Errorf("environment variable %q must be in user:password format", cfg.BasicEnv)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(val))
		cfg.Headers["Authorization"] = "Basic " + encoded
	}

	return nil
}
