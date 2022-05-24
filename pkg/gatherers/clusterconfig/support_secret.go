package clusterconfig

import (
	"context"
	"fmt"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// GatherSupportSecret gathers anonymized support secret if there is any
//
// * Location in archive: config/secrets/openshift-config/support/data.json
//     (can be omitted if the secret doesn't exist)
// * Id in config: support_secret
// * Since version:
//   * 4.12+
func (g *Gatherer) GatherSupportSecret(context.Context) ([]record.Record, []error) {
	if g.configObserver == nil {
		return nil, []error{fmt.Errorf("configObserver is nil")}
	}

	if supportSecret := g.configObserver.SupportSecret(); supportSecret != nil && supportSecret.Data != nil {
		return []record.Record{{
			Name: "config/secrets/openshift-config/support/data",
			Item: record.JSONMarshaller{Object: anonymizeSecretData(supportSecret.Data)},
		}}, nil
	}

	return nil, nil
}

func anonymizeSecretData(data map[string][]byte) map[string][]byte {
	if data == nil {
		return nil
	}

	if username, found := data["username"]; found {
		data["username"] = anonymize.Bytes(username)
	}
	if password, found := data["password"]; found {
		data["password"] = anonymize.Bytes(password)
	}

	// proxy potentially can have password inlined in it
	if httpProxy, found := data["httpProxy"]; found {
		data["httpProxy"] = anonymize.Bytes(httpProxy)
	}
	if httpsProxy, found := data["httpsProxy"]; found {
		data["httpsProxy"] = anonymize.Bytes(httpsProxy)
	}
	if noProxy, found := data["noProxy"]; found {
		data["noProxy"] = anonymize.Bytes(noProxy)
	}

	return data
}
