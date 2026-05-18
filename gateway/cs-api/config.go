package csapi

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// entityIDTokenRegex mirrors the per-token half of graph-ingest's
// entityIDRegex: alphanumeric start, alphanumeric / hyphen / underscore
// thereafter. The full 6-part regex is in the framework
// (processor/graph-ingest/component.go) but it is unexported, so we
// duplicate the per-token rule here for prefix validation.
var entityIDTokenRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Config tunes the cs-api gateway component. Defaults satisfy a development
// deployment; production deployments override via the JSON config file.
type Config struct {
	// BindAddress is used only in StandaloneServer mode. Production deployments
	// embed under semstreams' service.ServiceManager and leave it blank.
	BindAddress string `json:"bind_address"`

	// StandaloneServer makes the component manage its own *http.Server in
	// Start(). False is the production default — ServiceManager owns the
	// listener and calls RegisterHTTPHandlers on its shared mux.
	StandaloneServer bool `json:"standalone_server"`

	// QueryTimeout caps every NATS request/reply the gateway issues.
	// Tuned per ADR-S001's expectation that reads stay sub-second.
	QueryTimeout time.Duration `json:"query_timeout"`

	// ReadHeaderTimeout / ReadTimeout / WriteTimeout / IdleTimeout shape the
	// standalone HTTP server. Production deployments tune at the reverse
	// proxy instead.
	ReadHeaderTimeout time.Duration `json:"read_header_timeout"`
	ReadTimeout       time.Duration `json:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`

	// MaxRequestBytes caps request body size. POST endpoints will reject
	// larger bodies with 413.
	MaxRequestBytes int64 `json:"max_request_bytes"`

	// DefaultListLimit is the page size returned by collection endpoints
	// when the client did not pass ?limit=. CS API spec leaves this to the
	// implementation.
	DefaultListLimit int `json:"default_list_limit"`

	// MaxListLimit caps client-supplied ?limit= so a single request cannot
	// trigger an unbounded predicate scan.
	MaxListLimit int `json:"max_list_limit"`

	// PublishTimeout caps observation publishes (POST endpoints).
	// Kept separate from QueryTimeout so a slow JetStream ack does not
	// block independently-budgeted reads.
	PublishTimeout time.Duration `json:"publish_timeout"`

	// ObservationsStream is the JetStream stream name that captures
	// observation publishes. The cs-api gateway EnsureStream's it at
	// Start() so the first POST does not race the stream's creation.
	ObservationsStream string `json:"observations_stream"`

	// ObservationsSubjectPrefix shapes the per-observation publish subject.
	// A POST /datastreams/{id}/observations becomes
	// "<prefix>.<datastream_id>". Trailing dot is added automatically.
	ObservationsSubjectPrefix string `json:"observations_subject_prefix"`

	// ObservationsMaxAge bounds how long observation messages live in the
	// stream. 30 days is the v0.1 default — auditors get a window,
	// consumers that lag can replay. Tune per deployment.
	ObservationsMaxAge time.Duration `json:"observations_max_age"`

	// ObservationsMaxBytes is a soft cap on the stream's on-disk size. 0
	// means unlimited (today's behavior). Set in production so a runaway
	// client cannot fill the disk to the JetStream account limit.
	ObservationsMaxBytes int64 `json:"observations_max_bytes"`

	// ObservationsReplicas controls JetStream replica count. 1 is fine for
	// single-node dev/test; production HA uses 3.
	ObservationsReplicas int `json:"observations_replicas"`

	// SystemIDPrefix is the 5-part SemStreams entity ID prefix the gateway
	// uses when minting IDs for POST /systems. The 6th token is derived
	// from the SensorML uniqueId (or a UUID if uniqueId is absent).
	//
	// SemStreams entity IDs are exactly 6 dotted tokens
	// (org.platform.domain.system.type.instance per graph-ingest's
	// entityIDRegex). Validate() enforces that the prefix is exactly 5
	// tokens of valid characters; bad prefixes are a deployment-time
	// configuration error, not a runtime one.
	//
	// Default `c360.semconnect.systems.csapi.system` — operators set this
	// per deployment so cross-tenant entity IDs don't collide.
	SystemIDPrefix string `json:"system_id_prefix"`

	// DatastreamIDPrefix is the analogous 5-part prefix for POST /datastreams.
	// Defaults to `c360.semconnect.systems.csapi.datastream` so the 5th
	// token cleanly separates systems from datastreams under the same
	// deployment.
	DatastreamIDPrefix string `json:"datastream_id_prefix"`

	// ProcedureIDPrefix is the 5-part prefix for POST /procedures.
	// Stage 20 — separate from SystemIDPrefix so cross-tenant procedure
	// IDs land under a distinct 5th-token namespace.
	ProcedureIDPrefix string `json:"procedure_id_prefix"`

	// DeploymentIDPrefix is the 5-part prefix for POST /deployments.
	// Stage 21.
	DeploymentIDPrefix string `json:"deployment_id_prefix"`

	// SamplingFeatureIDPrefix is the 5-part prefix for POST /samplingFeatures.
	// Stage 22.
	SamplingFeatureIDPrefix string `json:"sampling_feature_id_prefix"`
}

// DefaultConfig returns a fully-populated Config. Stage 2 binaries call this
// and then overlay parsed JSON.
func DefaultConfig() Config {
	return Config{
		BindAddress:               ":8080",
		StandaloneServer:          false,
		QueryTimeout:              5 * time.Second,
		ReadHeaderTimeout:         5 * time.Second,
		ReadTimeout:               30 * time.Second,
		WriteTimeout:              60 * time.Second,
		IdleTimeout:               120 * time.Second,
		MaxRequestBytes:           1 << 20, // 1 MiB
		DefaultListLimit:          100,
		MaxListLimit:              1000,
		PublishTimeout:            5 * time.Second,
		ObservationsStream:        "CS_API_OBSERVATIONS",
		ObservationsSubjectPrefix: "cs-api.observations",
		ObservationsMaxAge:        30 * 24 * time.Hour,
		ObservationsMaxBytes:      0, // unlimited; set in prod
		ObservationsReplicas:      1,
		SystemIDPrefix:            "c360.semconnect.systems.csapi.system",
		DatastreamIDPrefix:        "c360.semconnect.systems.csapi.datastream",
		ProcedureIDPrefix:         "c360.semconnect.systems.csapi.procedure",
		DeploymentIDPrefix:        "c360.semconnect.systems.csapi.deployment",
		SamplingFeatureIDPrefix:   "c360.semconnect.systems.csapi.samplingfeature",
	}
}

// ApplyDefaults overlays zero-valued fields with DefaultConfig values.
// Order matters: caller parses JSON into a Config, then calls ApplyDefaults,
// then Validate.
func (c *Config) ApplyDefaults() {
	d := DefaultConfig()
	if c.BindAddress == "" {
		c.BindAddress = d.BindAddress
	}
	if c.QueryTimeout == 0 {
		c.QueryTimeout = d.QueryTimeout
	}
	if c.ReadHeaderTimeout == 0 {
		c.ReadHeaderTimeout = d.ReadHeaderTimeout
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = d.ReadTimeout
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = d.WriteTimeout
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = d.IdleTimeout
	}
	if c.MaxRequestBytes == 0 {
		c.MaxRequestBytes = d.MaxRequestBytes
	}
	if c.DefaultListLimit == 0 {
		c.DefaultListLimit = d.DefaultListLimit
	}
	if c.MaxListLimit == 0 {
		c.MaxListLimit = d.MaxListLimit
	}
	if c.PublishTimeout == 0 {
		c.PublishTimeout = d.PublishTimeout
	}
	if c.ObservationsStream == "" {
		c.ObservationsStream = d.ObservationsStream
	}
	if c.ObservationsSubjectPrefix == "" {
		c.ObservationsSubjectPrefix = d.ObservationsSubjectPrefix
	}
	if c.ObservationsMaxAge == 0 {
		c.ObservationsMaxAge = d.ObservationsMaxAge
	}
	if c.ObservationsReplicas == 0 {
		c.ObservationsReplicas = d.ObservationsReplicas
	}
	if c.SystemIDPrefix == "" {
		c.SystemIDPrefix = d.SystemIDPrefix
	}
	if c.DatastreamIDPrefix == "" {
		c.DatastreamIDPrefix = d.DatastreamIDPrefix
	}
	if c.ProcedureIDPrefix == "" {
		c.ProcedureIDPrefix = d.ProcedureIDPrefix
	}
	if c.DeploymentIDPrefix == "" {
		c.DeploymentIDPrefix = d.DeploymentIDPrefix
	}
	if c.SamplingFeatureIDPrefix == "" {
		c.SamplingFeatureIDPrefix = d.SamplingFeatureIDPrefix
	}
	// ObservationsMaxBytes: 0 is a meaningful value (unlimited); do not
	// overwrite with the default.
}

// Validate rejects nonsensical combinations. Called after ApplyDefaults.
func (c *Config) Validate() error {
	if c.QueryTimeout < 100*time.Millisecond {
		return errors.New("query_timeout below 100ms floor")
	}
	if c.QueryTimeout > 30*time.Second {
		return errors.New("query_timeout above 30s ceiling")
	}
	if c.MaxRequestBytes < 1024 {
		return errors.New("max_request_bytes must be ≥ 1024")
	}
	if c.DefaultListLimit < 1 {
		return errors.New("default_list_limit must be ≥ 1")
	}
	if c.MaxListLimit < c.DefaultListLimit {
		return errors.New("max_list_limit must be ≥ default_list_limit")
	}
	if c.StandaloneServer && c.BindAddress == "" {
		return errors.New("bind_address required when standalone_server is true")
	}
	if c.PublishTimeout < 100*time.Millisecond {
		return errors.New("publish_timeout below 100ms floor")
	}
	if c.PublishTimeout > 30*time.Second {
		return errors.New("publish_timeout above 30s ceiling")
	}
	if c.ObservationsStream == "" {
		return errors.New("observations_stream required")
	}
	if c.ObservationsSubjectPrefix == "" {
		return errors.New("observations_subject_prefix required")
	}
	if c.ObservationsMaxAge < time.Minute {
		return errors.New("observations_max_age must be ≥ 1 minute")
	}
	if c.ObservationsMaxBytes < 0 {
		return errors.New("observations_max_bytes must be ≥ 0")
	}
	if c.ObservationsReplicas < 1 || c.ObservationsReplicas > 5 {
		return errors.New("observations_replicas must be between 1 and 5")
	}
	if err := validateEntityIDPrefix(c.SystemIDPrefix, "system_id_prefix"); err != nil {
		return err
	}
	if err := validateEntityIDPrefix(c.ProcedureIDPrefix, "procedure_id_prefix"); err != nil {
		return err
	}
	if err := validateEntityIDPrefix(c.DeploymentIDPrefix, "deployment_id_prefix"); err != nil {
		return err
	}
	if err := validateEntityIDPrefix(c.SamplingFeatureIDPrefix, "sampling_feature_id_prefix"); err != nil {
		return err
	}
	if err := validateEntityIDPrefix(c.DatastreamIDPrefix, "datastream_id_prefix"); err != nil {
		return err
	}
	return nil
}

// validateEntityIDPrefix enforces that prefix is exactly 5 dotted tokens of
// SemStreams-valid characters ([a-zA-Z0-9][a-zA-Z0-9_-]*). The 6th token
// is appended at mint time per resource. Mismatched part count is the
// most common operator config error (4 parts, 6 parts) — fail loud at
// Validate() rather than producing invalid entity IDs at request time.
func validateEntityIDPrefix(prefix, fieldName string) error {
	if prefix == "" {
		return errors.New(fieldName + " required")
	}
	tokens := strings.Split(prefix, ".")
	if len(tokens) != 5 {
		return errors.New(fieldName + " must be exactly 5 dotted tokens (org.platform.domain.system.type)")
	}
	for i, tok := range tokens {
		if !entityIDTokenRegex.MatchString(tok) {
			return fmt.Errorf("%s token[%d]=%q not a valid SemStreams entity ID token", fieldName, i, tok)
		}
	}
	return nil
}
