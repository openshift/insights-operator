package gather

import (
	"context"

	"github.com/openshift/insights-operator/pkg/recorder"
)

type Interface interface {
	Gather(context.Context, []string, recorder.Interface) error
}
