package prof

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
)

// Config adds prof-specific configuration fields to the
// common cfg.Registerable and cfg.Validatable interfaces
type Config struct {
	EnablePyroscope      bool
	PyroServer           string
	PyroTenantID         string
	ProfileMutexFraction int
	BlockProfileRate     int
}

type Options struct {
	Enabled              bool
	AppName              string
	ServerAddress        string
	TenantID             string
	Tags                 map[string]string
	ProfileMutexFraction int
	BlockProfileRate     int
}

// RegisterFlags binds Config fields to the given FlagSet with defaults inline
func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.EnablePyroscope, "enable-pyroscope", false, "Enable pushing Pyroscope data to server set in -pyro-server")
	fs.StringVar(&c.PyroServer, "pyro-server", "", "pyroscope server url to push to")
	fs.StringVar(&c.PyroTenantID, "pyro-tenant", "", "tenant (x-scope-orgid) to use for pyro-server")
	fs.IntVar(&c.ProfileMutexFraction, "profile-mutex-fraction", 5, "mutex profiling sampling rate (0=disabled)")
	fs.IntVar(&c.BlockProfileRate, "profile-block-rate", 1000, "block profiling rate in nanoseconds (0=disabled)")
}

func (c *Config) Validate() error {
	var errs []error

	// Pyroscope (URL and scheme)
	if c.EnablePyroscope {
		if c.PyroServer == "" {
			errs = append(errs, fmt.Errorf("PYRO_SERVER required when ENABLE_PYROSCOPE=true"))
		} else if u, err := url.Parse(c.PyroServer); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("PYRO_SERVER must be a URL (got %q)", c.PyroServer))
		}
	}

	// Pyroscope tenant
	if c.EnablePyroscope && c.PyroTenantID == "" {
		errs = append(errs, fmt.Errorf("PYRO_TENANT required when ENABLE_PYROSCOPE=true"))
	}

	// Profile mutex fraction
	if c.ProfileMutexFraction < 0 {
		errs = append(errs, fmt.Errorf("PROFILE_MUTEX_FRACTION must be >= 0 (got %d)", c.ProfileMutexFraction))
	}

	// Block profile rate
	if c.BlockProfileRate < 0 {
		errs = append(errs, fmt.Errorf("PROFILE_BLOCK_RATE must be >= 0 (got %d)", c.BlockProfileRate))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *Config) ToOptions() *Options {

	return &Options{
		Enabled:              c.EnablePyroscope,
		ProfileMutexFraction: c.ProfileMutexFraction,
		BlockProfileRate:     c.BlockProfileRate,
		ServerAddress:        c.PyroServer,
		TenantID:             c.PyroTenantID,
	}
}
