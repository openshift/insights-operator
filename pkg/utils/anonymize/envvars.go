package anonymize

import (
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// SensitiveEnvVars finds env variables within the given container list
// and, if they are a target, it will obfuscate their value
func SensitiveEnvVars(containers []corev1.Container) {
	targets := []string{"HTTP_PROXY", "HTTPS_PROXY"}
	search := regexp.MustCompile(strings.Join(targets, "|"))

	for i := range containers {
		for j := range containers[i].Env {
			if search.MatchString(containers[i].Env[j].Name) {
				containers[i].Env[j].Value = String(containers[i].Env[j].Value)
			}
		}
	}
}
