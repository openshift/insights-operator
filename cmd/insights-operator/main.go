package main

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/client-go/pkg/version"
	"k8s.io/component-base/logs"

	"github.com/openshift/insights-operator/pkg/cmd/start"
)

func main() {
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	pflag.CommandLine.Lookup("alsologtostderr").Value.Set("true")

	logs.InitLogs()
	defer logs.FlushLogs()

	command := NewOperatorCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insights-operator",
		Short: "OpenShift Support Operator",

		SilenceUsage:  true,
		SilenceErrors: true,

		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(start.NewOperator())
	cmd.AddCommand(start.NewReceiver())

	return cmd
}
