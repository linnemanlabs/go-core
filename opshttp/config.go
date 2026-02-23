package opshttp

import (
	"errors"
	"flag"
	"fmt"
)

// Config adds opshttp-specific configuration fields to the
// common cfg.Registerable and cfg.Validatable interfaces
type Config struct {
	AdminPort   int
	EnablePprof bool
}

// RegisterFlags binds Config fields to the given FlagSet with defaults inline
func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.AdminPort, "admin-port", 9000, "admin listen TCP port (1..65535)")
	fs.BoolVar(&c.EnablePprof, "enable-pprof", true, "Enable pprof profiling (on admin port only)")

}

func (c *Config) Validate() error {
	var errs []error

	// Ports
	if c.AdminPort < 1 || c.AdminPort > 65535 {
		errs = append(errs, fmt.Errorf("invalid ADMIN_PORT %d (must be 1..65535)", c.AdminPort))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
