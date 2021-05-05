package workloads

// workloadPods is the top level description of the workloads on the cluster, primarily
// consisting of pod shapes by namespace. The shape of a pod is tracked as the content
// addressable hashes of each container image, a hash of the first command and argument,
// and hashes of the namespace name. This can be used to identify images that are publicly
// available but will not disclose details of private images such as names, content, or
// detailed metadata on the image. All identifying info is required to be hashed before
// sending - values such as "redis" or "/usr/bin/bash" could be reconstructed by comparing
// known hashes for those arguments.
//
// Additions to this data set are required to be reviewed for likelihood of data exposure
// and utility.
type workloadPods struct {
	// PodCount is the count of all pods scanned.
	PodCount int `json:"pods"`
	// ImageCount is the number of unique image IDs identified from pods.
	ImageCount int `json:"imageCount"`
	// Images is a map of image ID to data about the images referenced by pods. Images are
	// only populated if the cluster had imported the image ID to the image API via an
	// import or an image stream.
	Images map[string]workloadImage `json:"images"`
	// Namespaces is a map of namespace name hash to data about the namespace. The namespace
	// is populated even if it has no pods.
	Namespaces map[string]workloadNamespacePods `json:"namespaces"`
}

// workloadImage tracks a minimal set of metadata about images allowing identification
// of parent / child relationships via layers.
type workloadImage struct {
	// LayerIDs is the list of image layers in lowest-to-highest order.
	LayerIDs []string `json:"layerIDs"`
	// FirstCommand is a hash of the first value in the entrypoint array, if
	// any was set. Normalized to be consistent with pods.
	FirstCommand string `json:"firstCommand,omitempty"`
	// FirstArg is a hash of the first value in the command array, if any
	// was set. Normalized to be consistent with pods
	FirstArg string `json:"firstArg,omitempty"`
}

// Empty returns true if the image has no contents and can be ignored.
func (i workloadImage) Empty() bool {
	return len(i.LayerIDs) == 0
}

// workloadNamespacePods tracks the identified pod shapes within a namespace.
type workloadNamespacePods struct {
	// Count is the number of pods identified in the namespace.
	Count int `json:"count"`
	// TerminalCount is the number of pods that have reached a terminal phase
	// (success or error) in the namespace.
	TerminalCount int `json:"terminalCount,omitempty"`
	// IgnoredCount is the number of pods that are excluded because they are
	// in the terminal or unknown phases or have no pod status.
	IgnoredCount int `json:"ignoredCount,omitempty"`
	// InvalidCount is the number of pods that are returning partial information
	// about their shapes (no image ID in status) or cannot be evaluated at this
	// time.
	InvalidCount int `json:"invalidCount,omitempty"`
	// Shapes is the identified workload pod shapes in this namespace.
	Shapes []workloadPodShape `json:"shapes"`
}

// workloadPodShape describes a pod shape observed in a namespace. Pod shapes are
// identical if init containers and container shapes are identical.
type workloadPodShape struct {
	// Duplicates is the number of pods that share this shape. The number of
	// pods is always this number + one for the first pod with the shape.
	Duplicates int `json:"duplicates,omitempty"`

	// RestartAlways tracks whether a pod is a service (always restarts) or
	// a job (runs to completion).
	RestartsAlways bool `json:"restartAlways"`
	// InitContainers is the shapes of the init containers in this pod, in
	// the same order as they are defined in spec.
	InitContainers []workloadContainerShape `json:"initContainers,omitempty"`
	// Containers is the shapes of the containers in this pod, in
	// the same order as they are defined in spec.
	Containers []workloadContainerShape `json:"containers"`
}

// workloadContainerShape describes the shape of a container which includes
// a subset of the data in the container.
// TODO: this may desirable to make more precise with a whole container hash
//   that includes more of the workload, but that would only be necessary if
//   it assisted reconstruction of type of workloads.
type workloadContainerShape struct {
	// ImageID is the content addressable hash of the image as observed from
	// the status or the spec tag.
	ImageID string `json:"imageID"`
	// FirstCommand is a hash of the first value in the command array, if
	// any was set.
	FirstCommand string `json:"firstCommand,omitempty"`
	// FirstArg is a hash of the first value in the arguments array, if any
	// was set.
	FirstArg string `json:"firstArg,omitempty"`
}

type workloadImageInfo struct {
	count  int
	images map[string]workloadImage
}
