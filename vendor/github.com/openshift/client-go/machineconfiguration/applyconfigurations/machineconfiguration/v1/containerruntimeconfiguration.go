// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/openshift/api/machineconfiguration/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

// ContainerRuntimeConfigurationApplyConfiguration represents a declarative configuration of the ContainerRuntimeConfiguration type for use
// with apply.
type ContainerRuntimeConfigurationApplyConfiguration struct {
	PidsLimit      *int64                             `json:"pidsLimit,omitempty"`
	LogLevel       *string                            `json:"logLevel,omitempty"`
	LogSizeMax     *resource.Quantity                 `json:"logSizeMax,omitempty"`
	OverlaySize    *resource.Quantity                 `json:"overlaySize,omitempty"`
	DefaultRuntime *v1.ContainerRuntimeDefaultRuntime `json:"defaultRuntime,omitempty"`
}

// ContainerRuntimeConfigurationApplyConfiguration constructs a declarative configuration of the ContainerRuntimeConfiguration type for use with
// apply.
func ContainerRuntimeConfiguration() *ContainerRuntimeConfigurationApplyConfiguration {
	return &ContainerRuntimeConfigurationApplyConfiguration{}
}

// WithPidsLimit sets the PidsLimit field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the PidsLimit field is set to the value of the last call.
func (b *ContainerRuntimeConfigurationApplyConfiguration) WithPidsLimit(value int64) *ContainerRuntimeConfigurationApplyConfiguration {
	b.PidsLimit = &value
	return b
}

// WithLogLevel sets the LogLevel field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LogLevel field is set to the value of the last call.
func (b *ContainerRuntimeConfigurationApplyConfiguration) WithLogLevel(value string) *ContainerRuntimeConfigurationApplyConfiguration {
	b.LogLevel = &value
	return b
}

// WithLogSizeMax sets the LogSizeMax field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LogSizeMax field is set to the value of the last call.
func (b *ContainerRuntimeConfigurationApplyConfiguration) WithLogSizeMax(value resource.Quantity) *ContainerRuntimeConfigurationApplyConfiguration {
	b.LogSizeMax = &value
	return b
}

// WithOverlaySize sets the OverlaySize field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the OverlaySize field is set to the value of the last call.
func (b *ContainerRuntimeConfigurationApplyConfiguration) WithOverlaySize(value resource.Quantity) *ContainerRuntimeConfigurationApplyConfiguration {
	b.OverlaySize = &value
	return b
}

// WithDefaultRuntime sets the DefaultRuntime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the DefaultRuntime field is set to the value of the last call.
func (b *ContainerRuntimeConfigurationApplyConfiguration) WithDefaultRuntime(value v1.ContainerRuntimeDefaultRuntime) *ContainerRuntimeConfigurationApplyConfiguration {
	b.DefaultRuntime = &value
	return b
}
