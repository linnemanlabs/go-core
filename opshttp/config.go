package opshttp

import (
	"errors"
	"flag"
	"fmt"
)

// Config adds opshttp-specific configuration fields to the
// common cfg.Registerable and cfg.Validatable interfaces
type Config struct {
	Port        int
	EnablePprof bool
}

// RegisterFlags binds Config fields to the given FlagSet with defaults inline
func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.Port, "admin-port", 9000, "admin listen TCP port (1..65535)")
	fs.BoolVar(&c.EnablePprof, "admin-enable-pprof", true, "Enable pprof profiling (on admin port only)")
}

func (c *Config) Validate() error {
	var errs []error

	// Port must be in the valid TCP port range
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("invalid ADMIN_PORT %d (must be 1..65535)", c.Port))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *Config) ToOptions() *Options {
	return &Options{
		Port:        c.Port,
		EnablePprof: c.EnablePprof,
	}
}
