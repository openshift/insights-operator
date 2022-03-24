package dependencymagnet

// required for gomod to pull in packages.
// this is a dependency magnet to make it easier to pull in the build-machinery.  We want a single import to pull all of it in.
import (
	_ "github.com/openshift/build-machinery-go/make"
)
