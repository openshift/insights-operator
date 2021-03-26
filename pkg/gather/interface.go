package gather

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"
)

type Interface interface {
	Gather(context.Context, []string, record.Interface) error
}
