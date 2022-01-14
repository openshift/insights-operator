package clusterconfig

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/record"
)

func (g *Gatherer) GatherElasticsearch(ctx context.Context) ([]record.Record, []error) {
	conf := rest.CopyConfig(g.gatherProtoKubeConfig)
	conf.Impersonate.UserName = ""

	gatherKubeClient, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, []error{err}
	}

	coreClient := gatherKubeClient.CoreV1()

	secretList, err := coreClient.Secrets("openshift-insights").List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}

	secretToken := ""
	for _, s := range secretList.Items {
		if s.Type != "kubernetes.io/service-account-token" {
			continue
		}
		if saName, ok := s.ObjectMeta.Annotations["kubernetes.io/service-account.name"]; !ok || saName != "gather" {
			continue
		}
		secretTokenBytes, ok := s.Data["token"]
		if ok {
			secretToken = string(secretTokenBytes)
			break
		}
	}

	postData := strings.NewReader(`{
"query": {
	"query_string": {
	"query": "message:error AND (kubernetes.namespace_name:openshift-* OR kubernetes.namespace_name:kube*) AND NOT message:\"transport is closing\""
	}
}
}`)

	req, err := http.NewRequest(http.MethodPost, "https://elasticsearch.openshift-logging.svc:9200/_search", postData)
	if err != nil {
		return nil, []error{err}
	}
	req.Header.Set("Authorization", "Bearer "+secretToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.Do(req)
	if err != nil {
		return nil, []error{err}
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, []error{err}
	}

	return []record.Record{{Name: "config/elasticsearch", Item: RawJSON(respBytes)}}, nil
}
