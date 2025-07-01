
var _ = g.Describe("[sig-auth][Feature:OpenShiftAuthorization] ImageRegistry access", func() {
	defer g.GinkgoRecover()
	oc := exutil.NewCLI("bootstrap-policy")

	g.Context("", func() {
		g.Describe("PublicImageAccessWithBasicAuthShouldSucceed", func() {
			g.It("should succeed [apigroup:image.openshift.io]", func() {
 
				// Check if the image registry is enabled and available
				g.By("Validating image registry availability")
				registryEnabled, err := isImageRegistryAvailable(oc)
				o.Expect(err).NotTo(o.HaveOccurred(), "Failed to check if image registry is available")
				if !registryEnabled {
					g.Skip("Skipping test: Image registry is not available in this environment")
				}

				// Enable and use the default route for the image registry
				g.By("Enabling and using the default route for the image registry")
				err = enableDefaultRegistryRoute(oc)
				o.Expect(err).NotTo(o.HaveOccurred(), "Failed to enable the default registry route")

				host, err := getDefaultRegistryRoute(oc)
				o.Expect(err).NotTo(o.HaveOccurred(), "Failed to fetch the default registry route")
				o.Expect(host).NotTo(o.BeEmpty(), "Default registry route is empty")

				g.By("Fetch the latest image reference dynamically from the OpenShift namespace")
				// Run the command to get the latest image from the ImageStream
				output, err := oc.AsAdmin().Run("get").Args("imagestream", "tools", "-n", "openshift", "-o=jsonpath='{.status.tags[?(@.tag==\"latest\")].items[0].dockerImageReference}'").Output()
				o.Expect(err).NotTo(o.HaveOccurred(), "Failed to fetch the latest image reference from ImageStream")
				o.Expect(output).NotTo(o.BeEmpty(), "ImageStream output is empty")

				// Use the dynamically fetched image reference
				dynamicImage := output

				g.By("Try to fetch image metadata using the dynamic image reference")
				output, err = oc.AsAdmin().Run("image").Args("info", "--insecure", dynamicImage, "--show-multiarch").Output()

				o.Expect(err).NotTo(o.HaveOccurred(), "Failed to fetch image metadata")
				o.Expect(output).NotTo(o.ContainSubstring("error: unauthorized: authentication required"))
				o.Expect(output).NotTo(o.ContainSubstring("Unable to connect to the server: no basic auth credentials"))
				o.Expect(output).To(o.ContainSubstring(host + "/openshift/tools:latest"))
			})
		})
	})
})

// function to check if the image registry is available
func isImageRegistryAvailable(oc *exutil.CLI) (bool, error) {
	status, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o=jsonpath='{.spec.defaultRoute}'").Outputs()
	if err != nil {
		return false, fmt.Errorf("failed to check registry status: %v", err)
	}
	return status == "true", nil
}

// function to enable the default route for the image registry
func enableDefaultRegistryRoute(oc *exutil.CLI) error {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
		"configs.imageregistry.operator.openshift.io/cluster",
		"--patch", `{"spec":{"defaultRoute":true}}`,
		"--type=merge",
	).Execute()
	if err != nil {
		return fmt.Errorf("failed to enable default registry route: %v", err)
	}
	return nil
}

// function to fetch the default route for the image registry
func getDefaultRegistryRoute(oc *exutil.CLI) (string, error) {
	host, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configs.imageregistry.operator.openshift.io/cluster", "-o=jsonpath='{.status.defaultRoute}'").Outputs()
	if err != nil {
		return "", fmt.Errorf("failed to fetch default registry route: %v", err)
	}
	return host, nil
}

func exposeRouteFromSVC(oc *exutil.CLI, rType, ns, route, service string) string {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("route", rType, route, "--service="+service, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	regRoute, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", route, "-n", ns, "-o=jsonpath={.spec.host}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return regRoute
}

func waitRouteReady(route string) {
	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	curlCmd := "curl -k https://" + route
	var output []byte
	var curlErr error
	pollErr := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		output, curlErr = exec.Command("bash", "-c", curlCmd).CombinedOutput()
		if curlErr != nil {
			e2e.Logf("the route is not ready, go to next round")
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		e2e.Logf("output is: %v with error %v", string(output), curlErr.Error())
	}
	exutil.AssertWaitPollNoErr(pollErr, "The route can't be used")
}
