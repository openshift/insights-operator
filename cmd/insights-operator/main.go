package main

import (
	goflag "flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/pkg/version"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/cmd/start"
)

func main() {
	flags := goflag.CommandLine
	klog.InitFlags(flags)
	pflag.CommandLine.AddGoFlagSet(flags)
	err := pflag.CommandLine.Lookup("alsologtostderr").Value.Set("true")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logs.InitLogs()
	defer logs.FlushLogs()

	command := NewOperatorCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		defer os.Exit(1)
	}
}

func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "insights-operator",
		Short: "OpenShift Support Operator",

		SilenceUsage:  true,
		SilenceErrors: true,

		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
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
	cmd.AddCommand(start.NewGather())

	return cmd
}
