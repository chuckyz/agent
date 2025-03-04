package config

import (
	"fmt"
	"reflect"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	v1 "github.com/grafana/agent/pkg/integrations"
	v2 "github.com/grafana/agent/pkg/integrations/v2"
	"github.com/grafana/agent/pkg/metrics"
	"github.com/grafana/agent/pkg/server"
	"github.com/grafana/agent/pkg/util"
	"github.com/prometheus/statsd_exporter/pkg/level"
	"gopkg.in/yaml.v2"
)

type integrationsVersion int

const (
	integrationsVersion1 integrationsVersion = iota
	integrationsVersion2
)

// DefaultVersionedIntegrations is the default config for integrations.
var DefaultVersionedIntegrations = VersionedIntegrations{
	version:  integrationsVersion1,
	configV1: &v1.DefaultManagerConfig,
}

// VersionedIntegrations abstracts the subsystem configs for integrations v1
// and v2. VersionedIntegrations can only be unmarshaled as part of Load.
type VersionedIntegrations struct {
	version integrationsVersion
	raw     util.RawYAML

	configV1 *v1.ManagerConfig
	configV2 *v2.SubsystemOptions

	// ExtraIntegrations is used when adding any integrations NOT in the default agent configuration
	ExtraIntegrations []v2.Config
}

var (
	_ yaml.Unmarshaler = (*VersionedIntegrations)(nil)
	_ yaml.Marshaler   = (*VersionedIntegrations)(nil)
)

// UnmarshalYAML implements yaml.Unmarshaler. Full unmarshaling is deferred until
// setVersion is invoked.
func (c *VersionedIntegrations) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.configV1 = nil
	c.configV2 = nil
	return unmarshal(&c.raw)
}

// MarshalYAML implements yaml.Marshaler.
func (c VersionedIntegrations) MarshalYAML() (interface{}, error) {
	switch {
	case c.configV1 != nil:
		return c.configV1, nil
	case c.configV2 != nil:
		return c.configV2, nil
	default:
		// A pointer is needed for the yaml.Marshaler implementation to work.
		return &c.raw, nil
	}
}

// IsZero implements yaml.IsZeroer.
func (c VersionedIntegrations) IsZero() bool {
	switch {
	case c.configV1 != nil:
		return reflect.ValueOf(*c.configV1).IsZero()
	case c.configV2 != nil:
		return reflect.ValueOf(*c.configV2).IsZero()
	default:
		return len(c.raw) == 0
	}
}

// ApplyDefaults applies defaults to the subsystem based on globals.
func (c *VersionedIntegrations) ApplyDefaults(sflags *server.Flags, mcfg *metrics.Config) error {
	if c.version != integrationsVersion2 {
		return c.configV1.ApplyDefaults(sflags, mcfg)
	}
	return c.configV2.ApplyDefaults(mcfg)
}

// setVersion completes the deferred unmarshal and unmarshals the raw YAML into
// the subsystem config for version v.
func (c *VersionedIntegrations) setVersion(v integrationsVersion) error {
	c.version = v

	switch c.version {
	case integrationsVersion1:
		cfg := v1.DefaultManagerConfig
		c.configV1 = &cfg
		return yaml.UnmarshalStrict(c.raw, c.configV1)
	case integrationsVersion2:
		cfg := v2.DefaultSubsystemOptions
		// this is needed for dynamic configuration, the unmarshal doesnt work correctly if
		// this is not nil.
		c.configV1 = nil
		c.configV2 = &cfg
		err := yaml.UnmarshalStrict(c.raw, c.configV2)
		if err != nil {
			return err
		}
		c.configV2.Configs = append(c.configV2.Configs, c.ExtraIntegrations...)
		return nil
	default:
		panic(fmt.Sprintf("unknown integrations version %d", c.version))
	}
}

// IntegrationsGlobals is a global struct shared across integrations.
type IntegrationsGlobals = v2.Globals

// Integrations is an abstraction over both the v1 and v2 systems.
type Integrations interface {
	ApplyConfig(*VersionedIntegrations, IntegrationsGlobals) error
	WireAPI(*mux.Router)
	Stop()
}

// NewIntegrations creates a new subsystem. globals should be provided regardless
// of useV2. globals.SubsystemOptions will be automatically set if cfg.Version
// is set to IntegrationsVersion2.
func NewIntegrations(logger log.Logger, cfg *VersionedIntegrations, globals IntegrationsGlobals) (Integrations, error) {
	if cfg.version != integrationsVersion2 {
		instance, err := v1.NewManager(*cfg.configV1, logger, globals.Metrics.InstanceManager(), globals.Metrics.Validate)
		if err != nil {
			return nil, err
		}
		return &v1Integrations{Manager: instance}, nil
	}

	level.Warn(logger).Log("msg", "integrations-next is enabled. integrations-next is subject to change")

	globals.SubsystemOpts = *cfg.configV2
	instance, err := v2.NewSubsystem(logger, globals)
	if err != nil {
		return nil, err
	}
	return &v2Integrations{Subsystem: instance}, nil
}

type v1Integrations struct{ *v1.Manager }

func (s *v1Integrations) ApplyConfig(cfg *VersionedIntegrations, _ IntegrationsGlobals) error {
	return s.Manager.ApplyConfig(*cfg.configV1)
}

type v2Integrations struct{ *v2.Subsystem }

func (s *v2Integrations) ApplyConfig(cfg *VersionedIntegrations, globals IntegrationsGlobals) error {
	globals.SubsystemOpts = *cfg.configV2
	return s.Subsystem.ApplyConfig(globals)
}
