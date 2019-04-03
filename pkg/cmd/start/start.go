package start

import (
	"time"

	"github.com/spf13/cobra"

	"k8s.io/client-go/pkg/version"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/support-operator/pkg/controller"
)

func NewOperator() *cobra.Command {
	operator := &controller.Support{
		StoragePath: "/var/lib/support-operator",
		Interval:    10 * time.Minute,
		Endpoint:    "https://cloud.redhat.com/api/ingress/v1/upload",
	}
	cmd := controllercmd.NewControllerCommandConfig("openshift-support-operator", version.Get(), operator.Run).NewCommand()
	cmd.Use = "start"
	cmd.Short = "Start the operator"

	return cmd
}
