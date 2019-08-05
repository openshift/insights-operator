package gather

import (
	"context"

	"github.com/openshift/insights-operator/pkg/record"
)

type Interface interface {
	Gather(context.Context, record.Interface) error
}
