// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// ImageRegistryConfigStorageGCSApplyConfiguration represents an declarative configuration of the ImageRegistryConfigStorageGCS type for use
// with apply.
type ImageRegistryConfigStorageGCSApplyConfiguration struct {
	Bucket    *string `json:"bucket,omitempty"`
	Region    *string `json:"region,omitempty"`
	ProjectID *string `json:"projectID,omitempty"`
	KeyID     *string `json:"keyID,omitempty"`
}

// ImageRegistryConfigStorageGCSApplyConfiguration constructs an declarative configuration of the ImageRegistryConfigStorageGCS type for use with
// apply.
func ImageRegistryConfigStorageGCS() *ImageRegistryConfigStorageGCSApplyConfiguration {
	return &ImageRegistryConfigStorageGCSApplyConfiguration{}
}

// WithBucket sets the Bucket field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Bucket field is set to the value of the last call.
func (b *ImageRegistryConfigStorageGCSApplyConfiguration) WithBucket(value string) *ImageRegistryConfigStorageGCSApplyConfiguration {
	b.Bucket = &value
	return b
}

// WithRegion sets the Region field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Region field is set to the value of the last call.
func (b *ImageRegistryConfigStorageGCSApplyConfiguration) WithRegion(value string) *ImageRegistryConfigStorageGCSApplyConfiguration {
	b.Region = &value
	return b
}

// WithProjectID sets the ProjectID field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ProjectID field is set to the value of the last call.
func (b *ImageRegistryConfigStorageGCSApplyConfiguration) WithProjectID(value string) *ImageRegistryConfigStorageGCSApplyConfiguration {
	b.ProjectID = &value
	return b
}

// WithKeyID sets the KeyID field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the KeyID field is set to the value of the last call.
func (b *ImageRegistryConfigStorageGCSApplyConfiguration) WithKeyID(value string) *ImageRegistryConfigStorageGCSApplyConfiguration {
	b.KeyID = &value
	return b
}
