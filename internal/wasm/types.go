package wasm

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"

	_struct "google.golang.org/protobuf/types/known/structpb"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type Config struct {
	RequestData       map[string]string  `json:"requestData,omitempty"`
	Services          map[string]Service `json:"services"`
	ActionSets        []ActionSet        `json:"actionSets"`
	Observability     *Observability     `json:"observability,omitempty"`
	DescriptorService string             `json:"descriptorService,omitempty"`
}

func (c *Config) ToStruct() (*_struct.Struct, error) {
	configJSON, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	configStruct := &_struct.Struct{}
	if err := configStruct.UnmarshalJSON(configJSON); err != nil {
		return nil, err
	}
	return configStruct, nil
}

func (c *Config) ToJSON() (*apiextensionsv1.JSON, error) {
	configJSON, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	return &apiextensionsv1.JSON{Raw: configJSON}, nil
}

// EqualTo performs semantic equality comparison between two Config instances.
// This is not a strict equality check - it considers two configs equal if they are
// functionally equivalent, even if some collection orderings differ.
//
// Order-sensitive fields:
//   - ActionSets: Order matters - compared by index position
//   - RequestData: Map comparison (order doesn't apply)
//   - Services: Map comparison (order doesn't apply)
//   - DescriptorService: String comparison
//   - Observability: Strict equality via nested EqualTo
//
// Note: ActionSets order is significant because it affects the evaluation order
// in the data plane.
func (c *Config) EqualTo(other *Config) bool {
	if other == nil {
		return false
	}

	if len(c.RequestData) != len(other.RequestData) || len(c.Services) != len(other.Services) || len(c.ActionSets) != len(other.ActionSets) {
		return false
	}

	if c.DescriptorService != other.DescriptorService {
		return false
	}

	for key, data := range c.RequestData {
		if otherData, ok := other.RequestData[key]; !ok || data != otherData {
			return false
		}
	}

	for key, service := range c.Services {
		if otherService, ok := other.Services[key]; !ok || !service.EqualTo(otherService) {
			return false
		}
	}

	for i := range c.ActionSets {
		if !c.ActionSets[i].EqualTo(other.ActionSets[i]) {
			return false
		}
	}

	if (c.Observability == nil) != (other.Observability == nil) {
		return false
	}
	if c.Observability != nil && other.Observability != nil && !c.Observability.EqualTo(other.Observability) {
		return false
	}

	return true
}

type Service struct {
	Endpoint    string          `json:"endpoint"`
	Type        ServiceType     `json:"type"`
	FailureMode FailureModeType `json:"failureMode"`
	Timeout     *string         `json:"timeout,omitempty"`
	GrpcService *string         `json:"grpcService,omitempty"`
	GrpcMethod  *string         `json:"grpcMethod,omitempty"`
}

func (s Service) EqualTo(other Service) bool {
	if s.Endpoint != other.Endpoint ||
		s.Type != other.Type ||
		s.FailureMode != other.FailureMode ||
		((s.Timeout == nil) != (other.Timeout == nil)) ||
		(s.Timeout != nil && other.Timeout != nil && *s.Timeout != *other.Timeout) ||
		((s.GrpcService == nil) != (other.GrpcService == nil)) ||
		(s.GrpcService != nil && other.GrpcService != nil && *s.GrpcService != *other.GrpcService) ||
		((s.GrpcMethod == nil) != (other.GrpcMethod == nil)) ||
		(s.GrpcMethod != nil && other.GrpcMethod != nil && *s.GrpcMethod != *other.GrpcMethod) {
		return false
	}
	return true
}

// +kubebuilder:validation:Enum:=ratelimit;auth;ratelimit-check;ratelimit-report;tracing;dynamic
type ServiceType string

const (
	RateLimitServiceType       ServiceType = "ratelimit"
	RateLimitCheckServiceType  ServiceType = "ratelimit-check"
	RateLimitReportServiceType ServiceType = "ratelimit-report"
	AuthServiceType            ServiceType = "auth"
	TracingServiceType         ServiceType = "tracing"
	DynamicServiceType         ServiceType = "dynamic"
)

// +kubebuilder:validation:Enum:=deny;allow
type FailureModeType string

const (
	FailureModeDeny  FailureModeType = "deny"
	FailureModeAllow FailureModeType = "allow"
)

type ActionSet struct {
	Name string `json:"name"`

	// Conditions that activate the action set
	RouteRuleConditions RouteRuleConditions `json:"routeRuleConditions,omitempty"`

	// Actions that will be invoked when the conditions are met
	// +optional
	Actions []Action `json:"actions,omitempty"`
}

// EqualTo performs semantic equality comparison between two ActionSet instances.
//
// Order-sensitive fields:
//   - Name: String comparison
//   - RouteRuleConditions: Uses RouteRuleConditions.EqualTo (which has its own ordering rules)
//   - Actions: Order matters - compared by index position
//
// Note: Actions order is significant because it affects the evaluation order
// in the data plane.
func (s *ActionSet) EqualTo(other ActionSet) bool {
	if s.Name != other.Name || !s.RouteRuleConditions.EqualTo(other.RouteRuleConditions) || len(s.Actions) != len(other.Actions) {
		return false
	}

	for i := range s.Actions {
		if !s.Actions[i].EqualTo(other.Actions[i]) {
			return false
		}
	}

	return true
}

type RouteRuleConditions struct {
	Hostnames []string `json:"hostnames"`

	// +optional
	Predicates []string `json:"predicates,omitempty"`
}

// EqualTo performs semantic equality comparison between two RouteRuleConditions instances.
//
// Order-sensitive fields:
//   - Hostnames: Order matters - compared by index position
//   - Predicates: Order matters - compared by index position
//
// Note: Hostname order is preserved as it may reflect priority or specificity,
// while predicates are order-insensitive as they are evaluated independently.
func (r *RouteRuleConditions) EqualTo(other RouteRuleConditions) bool {
	if len(r.Hostnames) != len(other.Hostnames) || len(r.Predicates) != len(other.Predicates) {
		return false
	}

	for i := range r.Hostnames {
		if r.Hostnames[i] != other.Hostnames[i] {
			return false
		}
	}

	for i := range r.Predicates {
		if r.Predicates[i] != other.Predicates[i] {
			return false
		}
	}

	return true
}

type Action struct {
	ServiceName string `json:"service"`
	Scope       string `json:"scope"`

	Predicates []string `json:"predicates,omitempty"`

	// ConditionalData data contains the predicates and data that will be sent to the service
	// +optional
	ConditionalData []ConditionalData `json:"conditionalData,omitempty"`

	// SourcePolicyLocators tracks all policies that contributed to this action.
	// This is important for policies that can be merged (e.g., Gateway-level + HTTPRoute-level).
	// For atomic merge strategies or individual rules, this may contain a single entry.
	// Serialized to wasm config as "sources" for observability and debugging.
	// Format: "kind/namespace/name"
	SourcePolicyLocators []string `json:"sources,omitempty"`
}

type ConditionalData struct {
	// Predicates holds a list of CEL predicates
	// +optional
	Predicates []string `json:"predicates,omitempty"`

	// Data to be sent to the service
	// +optional
	Data []DataType `json:"data,omitempty"`
}

func (a *Action) HasAuthAccess() bool {
	for _, conditional := range a.ConditionalData {
		for _, predicate := range conditional.Predicates {
			if strings.Contains(predicate, "auth.") {
				return true
			}
		}
		for _, data := range conditional.Data {
			switch val := data.Value.(type) {
			case *Static:

				continue
			case *Expression:
				if strings.Contains(val.ExpressionItem.Value, "auth.") {
					return true
				}
			}
		}
	}
	return false
}

// EqualTo performs semantic equality comparison between two ConditionalData instances.
// Note: This has mixed ordering semantics for different fields.
//
// Order-sensitive fields:
//   - Predicates: Order matters - strict slice equality
//   - Data: Order matters - compared by index position
//
// Note: Both predicates and data order are significant as they may affect
// evaluation order in the data plane.
func (c *ConditionalData) EqualTo(other ConditionalData) bool {
	if len(c.Predicates) != len(other.Predicates) || len(c.Data) != len(other.Data) {
		return false
	}

	if !reflect.DeepEqual(c.Predicates, other.Predicates) {
		return false
	}

	for i := range c.Data {
		if !c.Data[i].EqualTo(other.Data[i]) {
			return false
		}
	}

	return true
}

// EqualTo performs semantic equality comparison between two Action instances.
// Note: This has mixed ordering semantics for different fields.
//
// Order-insensitive fields:
//   - ConditionalData: Checks that the same conditional data exists, regardless of order
//
// Order-sensitive fields (strict equality):
//   - Scope: String comparison
//   - ServiceName: String comparison
//   - Predicates: Strict slice equality - order matters
//   - SourcePolicyLocators: Strict slice equality - order matters
func (a *Action) EqualTo(other Action) bool {
	if a.Scope != other.Scope ||
		a.ServiceName != other.ServiceName ||
		len(a.ConditionalData) != len(other.ConditionalData) {
		return false
	}

	if !reflect.DeepEqual(a.Predicates, other.Predicates) {
		return false
	}

	if !slices.Equal(a.SourcePolicyLocators, other.SourcePolicyLocators) {
		return false
	}

	for i := range a.ConditionalData {
		if !slices.ContainsFunc(other.ConditionalData, a.ConditionalData[i].EqualTo) {
			return false
		}
	}

	return true
}

type DataType struct {
	Value any
}

func (d *DataType) UnmarshalJSON(data []byte) error {
	// Precisely one of "static", "selector" must be set.
	types := []any{
		&Static{},
		&Expression{},
	}

	var err error

	for idx := range types {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields() // Force errors
		err = dec.Decode(types[idx])
		if err == nil {
			d.Value = types[idx]
			return nil
		}
	}

	return err
}

func (d *DataType) MarshalJSON() ([]byte, error) {
	switch val := d.Value.(type) {
	case *Static:
		return json.Marshal(val)
	case *Expression:
		return json.Marshal(val)
	default:
		return nil, errors.New("DataType.Value has unknown type")
	}
}

func (d *DataType) EqualTo(other DataType) bool {
	dt, err := d.MarshalJSON()
	if err != nil {
		return false
	}
	odt, err := other.MarshalJSON()
	if err != nil {
		return false
	}
	return bytes.Equal(dt, odt)
}

type Static struct {
	Static StaticSpec `json:"static"`
}

type StaticSpec struct {
	Value string `json:"value"`
	Key   string `json:"key"`
}

type ExpressionItem struct {
	// Key holds the expression key
	Key string `json:"key"`

	// Value holds the CEL expression
	Value string `json:"value"`
}

type Expression struct {
	// Data to be sent to the service
	ExpressionItem ExpressionItem `json:"expression"`
}

type Observability struct {
	HTTPHeaderIdentifier *string  `json:"httpHeaderIdentifier,omitempty"`
	DefaultLevel         *string  `json:"defaultLevel,omitempty"`
	Tracing              *Tracing `json:"tracing,omitempty"`
}

type Tracing struct {
	Service string `json:"service,omitempty"`
}

func (o *Observability) EqualTo(other *Observability) bool {
	if o == nil && other == nil {
		return true
	}
	if o == nil || other == nil {
		return false
	}

	if (o.HTTPHeaderIdentifier == nil) != (other.HTTPHeaderIdentifier == nil) {
		return false
	}
	if o.HTTPHeaderIdentifier != nil && other.HTTPHeaderIdentifier != nil && *o.HTTPHeaderIdentifier != *other.HTTPHeaderIdentifier {
		return false
	}

	if (o.DefaultLevel == nil) != (other.DefaultLevel == nil) {
		return false
	}
	if o.DefaultLevel != nil && other.DefaultLevel != nil && *o.DefaultLevel != *other.DefaultLevel {
		return false
	}

	if (o.Tracing == nil) != (other.Tracing == nil) {
		return false
	}
	if o.Tracing != nil && other.Tracing != nil && !o.Tracing.EqualTo(other.Tracing) {
		return false
	}

	return true
}

func (t *Tracing) EqualTo(other *Tracing) bool {
	if t == nil && other == nil {
		return true
	}
	if t == nil || other == nil {
		return false
	}
	return t.Service == other.Service
}
