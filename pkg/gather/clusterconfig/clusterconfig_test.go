package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	networkv1 "github.com/openshift/api/network/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apixv1beta1clientfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog"

	configfake "github.com/openshift/client-go/config/clientset/versioned/fake"
	imageregistryfake "github.com/openshift/client-go/imageregistry/clientset/versioned/fake"
	networkfake "github.com/openshift/client-go/network/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

func TestConfigMapAnonymizer(t *testing.T) {
	klog.SetOutput(utils.NewTestLog(t).Writer())

	var cases = []struct {
		testName               string
		configMapName          string
		expectedAnonymizedJSON string
	}{
		{
			"ConfigMap Non PEM data",
			"openshift-install",
			`{
				"invoker":"codeReadyContainers",
				"version":"unreleased-master-2205-g2055609f95b19322ee6cfdd0bea73399297c4a3e"
			}`,
		},
		{
			"ConfigMap PEM is anonymized",
			"initial-kube-apiserver-server-ca",
			`{
				"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n"
			}`,
		},
		{
			"ConfigMap BinaryData non anonymized",
			"test-binary",
			`{
				"ls": "z/rt/gcAAAEDAA=="
			}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.testName, func(t *testing.T) {
			f, err := os.Open("testdata/configmaps.json")
			mustNotFail(t, err, "error opening test data file. %+v")
			defer f.Close()
			bts, err := ioutil.ReadAll(f)
			mustNotFail(t, err, "error reading test data file. %+v")
			var cml *corev1.ConfigMapList
			mustNotFail(t, json.Unmarshal([]byte(bts), &cml), "error unmarshalling json %+v")
			cm := findMap(cml, tt.configMapName)
			mustNotFail(t, cm != nil, "haven't found a ConfigMap %+v")
			var res []byte
			cmdata := map[string]string{}
			addAnonymized := func(cmdata map[string]string, dn string, encodebase64 bool, d []byte) {
				m := record.Marshalable(ConfigMapAnonymizer{v: d, encodeBase64: encodebase64})

				res, err = m.Marshal(context.TODO())
				cmdata[dn] = string(res)
				mustNotFail(t, err, "serialization failed %+v")
			}
			for dn, dv := range cm.Data {
				addAnonymized(cmdata, dn, false, []byte(dv))
			}
			for dn, dv := range cm.BinaryData {
				addAnonymized(cmdata, dn, true, dv)
			}
			var md []byte
			md, err = json.Marshal(cmdata)
			mustNotFail(t, err, "marshaling failed %+v")
			d := map[string]string{}
			err = json.Unmarshal([]byte(tt.expectedAnonymizedJSON), &d)
			mustNotFail(t, err, "unmarshaling of expected failed %+v")
			exp, err := json.Marshal(d)
			mustNotFail(t, err, "marshaling of expected failed %+v")
			if string(exp) != string(md) {
				t.Fatalf("The test %s result is unexpected. Result: \n%s \nExpected \n%s", tt.testName, string(md), string(exp))
			}
		})
	}

}

func TestGatherClusterPruner(t *testing.T) {
	tests := []struct {
		name            string
		inputObj        runtime.Object
		expectedRecords int
		evalOutput      func(t *testing.T, obj *imageregistryv1.ImagePruner)
	}{
		{
			name: "not found",
			inputObj: &imageregistryv1.ImagePruner{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pruner-i-dont-care-about",
				},
			},
		},
		{
			name:            "simple image pruner",
			expectedRecords: 1,
			inputObj: &imageregistryv1.ImagePruner{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImagePrunerSpec{
					Schedule: "0 0 * * *",
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.ImagePruner) {
				if obj.Name != "cluster" {
					t.Errorf("received wrong prunner: %+v", obj)
					return
				}
				if obj.Spec.Schedule != "0 0 * * *" {
					t.Errorf("unexpected spec.schedule: %q", obj.Spec.Schedule)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := imageregistryfake.NewSimpleClientset(test.inputObj)
			gatherer := &Gatherer{registryClient: client.ImageregistryV1()}
			records, errs := GatherClusterImagePruner(gatherer)()
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if numRecords := len(records); numRecords != test.expectedRecords {
				t.Errorf("expected one record, got %d", numRecords)
				return
			}
			if test.expectedRecords == 0 {
				return
			}
			if expectedRecordName := "config/imagepruner"; records[0].Name != expectedRecordName {
				t.Errorf("expected %q record name, got %q", expectedRecordName, records[0].Name)
				return
			}
			item := records[0].Item
			itemBytes, err := item.Marshal(context.TODO())
			if err != nil {
				t.Fatalf("unable to marshal config: %v", err)
			}
			var output imageregistryv1.ImagePruner
			obj, _, err := registrySerializer.LegacyCodec(imageregistryv1.SchemeGroupVersion).Decode(itemBytes, nil, &output)
			if err != nil {
				t.Fatalf("failed to decode object: %v", err)
			}
			test.evalOutput(t, obj.(*imageregistryv1.ImagePruner))
		})
	}
}

func TestGatherPodDisruptionBudgets(t *testing.T) {
	coreClient := kubefake.NewSimpleClientset()

	fakeNamespace := "fake-namespace"

	// name -> MinAvailabel
	fakePDBs := map[string]string{
		"pdb-four":  "4",
		"pdb-eight": "8",
		"pdb-ten":   "10",
	}
	for name, minAvailable := range fakePDBs {
		_, err := coreClient.PolicyV1beta1().
			PodDisruptionBudgets(fakeNamespace).
			Create(context.Background(), &policyv1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: fakeNamespace,
					Name:      name,
				},
				Spec: policyv1beta1.PodDisruptionBudgetSpec{
					MinAvailable: &intstr.IntOrString{StrVal: minAvailable},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("unable to create fake pdbs: %v", err)
		}
	}

	gatherer := &Gatherer{policyClient: coreClient.PolicyV1beta1()}

	records, errs := GatherPodDisruptionBudgets(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != len(fakePDBs) {
		t.Fatalf("unexpected number of records gathered: %d (expected %d)", len(records), len(fakePDBs))
	}
	for _, rec := range records {
		pdba, ok := rec.Item.(PodDisruptionBudgetsAnonymizer)
		if !ok {
			t.Fatal("pdb item has invalid type")
		}
		name := pdba.PodDisruptionBudget.ObjectMeta.Name
		minAvailable := pdba.PodDisruptionBudget.Spec.MinAvailable.StrVal
		if pdba.PodDisruptionBudget.Spec.MinAvailable.StrVal != fakePDBs[name] {
			t.Fatalf("pdb item has mismatched MinAvailable value, %q != %q", fakePDBs[name], minAvailable)
		}
	}
}

func TestGatherClusterImageRegistry(t *testing.T) {
	tests := []struct {
		name       string
		inputObj   *imageregistryv1.Config
		evalOutput func(t *testing.T, obj *imageregistryv1.Config)
	}{
		{
			name: "httpSecret",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					HTTPSecret: "secret",
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.HTTPSecret != "xxxxxx" {
					t.Errorf("expected HTTPSecret anonymized, got %q", obj.Spec.HTTPSecret)
				}
			},
		},
		{
			name: "s3",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						S3: &imageregistryv1.ImageRegistryConfigStorageS3{
							Bucket:         "foo",
							Region:         "bar",
							RegionEndpoint: "point",
							KeyID:          "key",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.S3.Bucket != "xxx" {
					t.Errorf("expected s3 bucket anonymized, got %q", obj.Spec.Storage.S3.Bucket)
				}
				if obj.Spec.Storage.S3.Region != "xxx" {
					t.Errorf("expected s3 region anonymized, got %q", obj.Spec.Storage.S3.Region)
				}
				if obj.Spec.Storage.S3.RegionEndpoint != "xxxxx" {
					t.Errorf("expected s3 region endpoint anonymized, got %q", obj.Spec.Storage.S3.RegionEndpoint)
				}
				if obj.Spec.Storage.S3.KeyID != "xxx" {
					t.Errorf("expected s3 keyID anonymized, got %q", obj.Spec.Storage.S3.KeyID)
				}
			},
		},
		{
			name: "azure",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						Azure: &imageregistryv1.ImageRegistryConfigStorageAzure{
							AccountName: "account",
							Container:   "container",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.Azure.AccountName != "xxxxxxx" {
					t.Errorf("expected azure account name anonymized, got %q", obj.Spec.Storage.Azure.AccountName)
				}
				if obj.Spec.Storage.Azure.Container == "xxxxxxx" {
					t.Errorf("expected azure container anonymized, got %q", obj.Spec.Storage.Azure.Container)
				}
			},
		},
		{
			name: "gcs",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						GCS: &imageregistryv1.ImageRegistryConfigStorageGCS{
							Bucket:    "bucket",
							Region:    "region",
							ProjectID: "foo",
							KeyID:     "bar",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.GCS.Bucket != "xxxxxx" {
					t.Errorf("expected gcs bucket anonymized, got %q", obj.Spec.Storage.GCS.Bucket)
				}
				if obj.Spec.Storage.GCS.ProjectID != "xxx" {
					t.Errorf("expected gcs projectID endpoint anonymized, got %q", obj.Spec.Storage.GCS.ProjectID)
				}
				if obj.Spec.Storage.GCS.KeyID != "xxx" {
					t.Errorf("expected gcs keyID anonymized, got %q", obj.Spec.Storage.GCS.KeyID)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := imageregistryfake.NewSimpleClientset(test.inputObj)
			gatherer := &Gatherer{registryClient: client.ImageregistryV1()}
			records, errs := GatherClusterImageRegistry(gatherer)()
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if numRecords := len(records); numRecords != 1 {
				t.Errorf("expected one record, got %d", numRecords)
				return
			}
			if expectedRecordName := "config/imageregistry"; records[0].Name != expectedRecordName {
				t.Errorf("expected %q record name, got %q", expectedRecordName, records[0].Name)
				return
			}
			item := records[0].Item
			itemBytes, err := item.Marshal(context.TODO())
			if err != nil {
				t.Fatalf("unable to marshal config: %v", err)
			}
			var output imageregistryv1.Config
			obj, _, err := registrySerializer.LegacyCodec(imageregistryv1.SchemeGroupVersion).Decode(itemBytes, nil, &output)
			if err != nil {
				t.Fatalf("failed to decode object: %v", err)
			}
			test.evalOutput(t, obj.(*imageregistryv1.Config))
		})
	}
}

func TestGatherContainerImages(t *testing.T) {
	const fakeNamespace = "fake-namespace"
	const fakeOpenshiftNamespace = "openshift-fake-namespace"

	mockContainers := []string{
		"registry.redhat.io/1",
		"registry.redhat.io/2",
		"registry.redhat.io/3",
	}

	expected := ContainerInfo{
		Images: ContainerImageSet{
			0: "registry.redhat.io/1",
			1: "registry.redhat.io/2",
			2: "registry.redhat.io/3",
		},
		Containers: PodsWithAge{
			"0001-01": RunningImages{
				0: 1,
				1: 1,
				2: 1,
			},
		},
	}

	coreClient := kubefake.NewSimpleClientset()
	for index, containerImage := range mockContainers {
		_, err := coreClient.CoreV1().
			Pods(fakeNamespace).
			Create(context.Background(), &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: fakeNamespace,
					Name:      fmt.Sprintf("pod%d", index),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  fmt.Sprintf("container%d", index),
							Image: containerImage,
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake pod")
		}
	}

	const numberOfCrashlooping = 10
	expectedRecords := make([]string, numberOfCrashlooping)
	for i := 0; i < numberOfCrashlooping; i++ {
		podName := fmt.Sprintf("crashlooping%d", i)
		_, err := coreClient.CoreV1().
			Pods(fakeOpenshiftNamespace).
			Create(context.Background(), &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: int32(numberOfCrashlooping - i),
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: int32(i + 1),
								},
							},
						},
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake pod")
		}
		expectedRecords[i] = fmt.Sprintf("config/pod/%s/%s", fakeOpenshiftNamespace, podName)
	}

	gatherer := &Gatherer{coreClient: coreClient.CoreV1()}

	records, errs := GatherContainerImages(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	var containerInfo *ContainerInfo = nil
	for _, rec := range records {
		if rec.Name == "config/running_containers" {
			anonymizer, ok := rec.Item.(record.JSONMarshaller)
			if !ok {
				t.Fatal("reported running containers item has invalid type")
			}

			containers, ok := anonymizer.Object.(ContainerInfo)
			if !ok {
				t.Fatal("anonymized running containers data have wrong type")
			}

			containerInfo = &containers
		}
	}

	if containerInfo == nil {
		t.Fatal("container info has not been reported")
	}

	if !reflect.DeepEqual(*containerInfo, expected) {
		t.Fatalf("unexpected result: %#v", *containerInfo)
	}

	for _, expectedRecordName := range expectedRecords {
		wasReported := false
		for _, reportedRecord := range records {
			if reportedRecord.Name == expectedRecordName {
				wasReported = true
				break
			}
		}
		if !wasReported {
			t.Fatalf("expected record '%s' was not reported", expectedRecordName)
		}
	}
}

func TestCollectVolumeSnapshotCRD(t *testing.T) {
	expectedRecords := map[string]v1beta1.CustomResourceDefinition{
		"config/crd/volumesnapshots.snapshot.storage.k8s.io":        {ObjectMeta: metav1.ObjectMeta{Name: "volumesnapshots.snapshot.storage.k8s.io"}},
		"config/crd/volumesnapshotcontents.snapshot.storage.k8s.io": {ObjectMeta: metav1.ObjectMeta{Name: "volumesnapshotcontents.snapshot.storage.k8s.io"}},
	}

	crdNames := []string{
		"unrelated.custom.resource.definition.k8s.io",
		"volumesnapshots.snapshot.storage.k8s.io",
		"volumesnapshotcontents.snapshot.storage.k8s.io",
		"another.irrelevant.custom.resource.definition.k8s.io",
		"this.should.not.be.gathered.k8s.io",
	}

	crdClientset := apixv1beta1clientfake.NewSimpleClientset()

	for _, name := range crdNames {
		crdClientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(context.Background(), &v1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}, metav1.CreateOptions{})
	}

	gatherer := &Gatherer{crdClient: crdClientset.ApiextensionsV1beta1()}
	records, errs := GatherCRD(gatherer)()
	if len(errs) != 0 {
		t.Fatalf("gather CRDs resulted in error: %#v", errs)
	}

	if len(records) != len(expectedRecords) {
		t.Fatalf("unexpected number of records gathered: %d (expected %d)", len(records), len(expectedRecords))
	}

	for _, rec := range records {
		if expectedItem, ok := expectedRecords[rec.Name]; !ok {
			t.Fatalf("unexpected gathered record name: %q", rec.Name)
		} else if reflect.DeepEqual(rec.Item, expectedItem) {
			t.Fatalf("gathered record %q has different item value than unexpected", rec.Name)
		}
	}
}

func TestGatherHostSubnet(t *testing.T) {
	testHostSubnet := networkv1.HostSubnet{
		Host:        "test.host",
		HostIP:      "10.0.0.0",
		Subnet:      "10.0.0.0/23",
		EgressIPs:   []networkv1.HostSubnetEgressIP{"10.0.0.0", "10.0.0.1"},
		EgressCIDRs: []networkv1.HostSubnetEgressCIDR{"10.0.0.0/24", "10.0.0.0/24"},
	}
	client := networkfake.NewSimpleClientset()
	_, err := client.NetworkV1().HostSubnets().Create(context.Background(), &testHostSubnet, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake hostsubnet")
	}

	gatherer := &Gatherer{networkClient: client.NetworkV1()}

	records, errs := GatherHostSubnet(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	item, err := records[0].Item.Marshal(context.TODO())
	var gatheredHostSubnet networkv1.HostSubnet
	_, _, err = networkSerializer.LegacyCodec(networkv1.SchemeGroupVersion).Decode(item, nil, &gatheredHostSubnet)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredHostSubnet.HostIP != "xxxxxxxx" {
		t.Fatalf("Host IP is not anonymized %s", gatheredHostSubnet.HostIP)
	}
	if gatheredHostSubnet.Subnet != "xxxxxxxxxxx" {
		t.Fatalf("Host Subnet is not anonymized %s", gatheredHostSubnet.Subnet)
	}
	if len(gatheredHostSubnet.EgressIPs) != len(testHostSubnet.EgressIPs) {
		t.Fatalf("unexpected number of egress IPs gathered %s", gatheredHostSubnet.EgressIPs)
	}

	if len(gatheredHostSubnet.EgressCIDRs) != len(testHostSubnet.EgressCIDRs) {
		t.Fatalf("unexpected number of egress CIDRs gathered %s", gatheredHostSubnet.EgressCIDRs)
	}

	for _, ip := range gatheredHostSubnet.EgressIPs {
		if ip != "xxxxxxxx" {
			t.Fatalf("Egress IP is not anonymized %s", ip)
		}
	}

	for _, cidr := range gatheredHostSubnet.EgressCIDRs {
		if cidr != "xxxxxxxxxxx" {
			t.Fatalf("Egress CIDR is not anonymized %s", cidr)
		}
	}
}

func TestGatherMachineSet(t *testing.T) {
	var machineSetYAML = `
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
    name: test-worker
`
	gvr := schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineSet := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineSetYAML), nil, testMachineSet)
	if err != nil {
		t.Fatal("unable to decode machineset ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachineSet, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineset ", err)
	}

	gatherer := &Gatherer{dynamicClient: client}
	records, errs := GatherMachineSet(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "machinesets/test-worker" {
		t.Fatalf("unexpected machineset name %s", records[0].Name)
	}
}

func TestGatherInstallPlans(t *testing.T) {
	tests := []struct {
		name      string
		testfiles []string
		limit     int
		exp       string
	}{
		{
			name:      "one installplan",
			testfiles: []string{"testdata/installplan.yaml"},
			exp: `{"items":[{"count":1,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],` +
				`"stats":{"TOTAL_COUNT":1,"TOTAL_NONUNIQ_COUNT":1}}`,
		},
		{
			name:      "two are same to keep ordering and one is different",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml", "testdata/installplan_openshift.yaml"},
			exp:       `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"},{"count":1,"csv":"3scale-community-operator.v0.5.1","name":"install-","ns":"openshift"}],"stats":{"TOTAL_COUNT":3,"TOTAL_NONUNIQ_COUNT":2}}`,
		},
		{
			name:      "two similar installplans",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml"},
			exp: `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],` +
				`"stats":{"TOTAL_COUNT":2,"TOTAL_NONUNIQ_COUNT":1}}`,
		},
		{
			name:      "test marshaller with limit to 1 item",
			testfiles: []string{"testdata/installplan.yaml", "testdata/installplan2.yaml", "testdata/installplan_openshift.yaml"},
			limit:     1,
			exp:       `{"items":[{"count":2,"csv":"lib-bucket-provisioner.v2.0.0","name":"install-","ns":"openshift-operators"}],"stats":{"TOTAL_COUNT":3,"TOTAL_NONUNIQ_COUNT":2}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			coreClient := kubefake.NewSimpleClientset()
			for _, file := range test.testfiles {
				f, err := os.Open(file)
				if err != nil {
					t.Fatal("test failed to read installplan data", err)
				}
				defer f.Close()
				installplancontent, err := ioutil.ReadAll(f)
				if err != nil {
					t.Fatal("error reading test data file", err)
				}

				decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
				installplan := &unstructured.Unstructured{}

				_, _, err = decUnstructured.Decode(installplancontent, nil, installplan)
				if err != nil {
					t.Fatal("unable to decode", err)
				}
				gv, _ := schema.ParseGroupVersion(installplan.GetAPIVersion())
				gvr := schema.GroupVersionResource{Version: gv.Version, Group: gv.Group, Resource: "installplans"}
				var ns string
				err = parseJSONQuery(installplan.Object, "metadata.namespace", &ns)
				if err != nil {
					t.Fatal("unable to read ns ", err)
				}
				_, err = coreClient.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					_, err = coreClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
				}
				if err != nil {
					t.Fatal("unable to create ns fake ", err)
				}
				_, err = client.Resource(gvr).Namespace(ns).Create(context.Background(), installplan, metav1.CreateOptions{})
				if err != nil {
					t.Fatal("unable to create installplan fake ", err)
				}
			}

			gatherer := &Gatherer{ctx: context.Background(), dynamicClient: client, coreClient: coreClient.CoreV1()}
			records, errs := GatherInstallPlans(gatherer)()
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if len(records) != 1 {
				t.Fatalf("unexpected number or records %d", len(records))
			}
			m, ok := records[0].Item.(InstallPlanAnonymizer)
			if !ok {
				t.Fatalf("returned item is not of type InstallPlanAnonymizer")
			}
			if test.limit != 0 {
				// copy to new anonymizer with limited max
				m = InstallPlanAnonymizer{limit: 1, total: m.total, v: m.v}
			}
			b, _ := m.Marshal(context.Background())
			sb := string(b)
			if sb != test.exp {
				t.Fatalf("unexpected installplan exp: %s got: %s", test.exp, sb)
			}
		})

	}
}

func TestGatherMachineConfigPool(t *testing.T) {
	var machineconfigpoolYAML = `
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigPool
metadata:
    name: master-t
`
	gvr := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testMachineConfigPools := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineconfigpoolYAML), nil, testMachineConfigPools)
	if err != nil {
		t.Fatal("unable to decode machineconfigpool ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testMachineConfigPools, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineconfigpool ", err)
	}

	gatherer := &Gatherer{dynamicClient: client}
	records, errs := GatherMachineConfigPool(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/machineconfigpools/master-t" {
		t.Fatalf("unexpected machineconfigpool name %s", records[0].Name)
	}
}

func TestContainerRuntimeConfig(t *testing.T) {
	var machineconfigpoolYAML = `
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
    name: test-ContainerRC
`
	gvr := schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "containerruntimeconfigs"}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	testContainerRuntimeConfigs := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(machineconfigpoolYAML), nil, testContainerRuntimeConfigs)
	if err != nil {
		t.Fatal("unable to decode machineconfigpool ", err)
	}
	_, err = client.Resource(gvr).Create(context.Background(), testContainerRuntimeConfigs, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake machineconfigpool ", err)
	}

	gatherer := &Gatherer{dynamicClient: client}
	records, errs := GatherContainerRuntimeConfig(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 1 {
		t.Fatalf("unexpected number or records %d", len(records))
	}
	if records[0].Name != "config/containerruntimeconfigs/test-ContainerRC" {
		t.Fatalf("unexpected containerruntimeconfig name %s", records[0].Name)
	}
}

func TestGatherServiceAccounts(t *testing.T) {
	tests := []struct {
		name string
		data []*corev1.ServiceAccount
		exp  string
	}{
		{
			name: "one account",
			data: []*corev1.ServiceAccount{&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-storage-operator",
					Namespace: "default",
				},
				Secrets: []corev1.ObjectReference{corev1.ObjectReference{}},
			}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":1,"namespaces":{"default":{"name":"local-storage-operator","secrets":1}}}}`,
		},
		{
			name: "multiple accounts",
			data: []*corev1.ServiceAccount{&corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployer",
					Namespace: "openshift",
				},
				Secrets: []corev1.ObjectReference{corev1.ObjectReference{}},
			},
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openshift-apiserver-sa",
						Namespace: "openshift-apiserver",
					},
					Secrets: []corev1.ObjectReference{corev1.ObjectReference{}},
				}},
			exp: `{"serviceAccounts":{"TOTAL_COUNT":2,"namespaces":{"openshift":{"name":"deployer","secrets":1},"openshift-apiserver":{"name":"openshift-apiserver-sa","secrets":1}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreClient := kubefake.NewSimpleClientset()
			for _, d := range test.data {
				_, err := coreClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: d.Namespace}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("unable to create fake ns %s", err)
				}
				_, err = coreClient.CoreV1().ServiceAccounts(d.Namespace).
					Create(context.Background(), d, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("unable to create fake service account %s", err)
				}
			}
			gatherer := &Gatherer{ctx: context.Background(), coreClient: coreClient.CoreV1()}
			sa, errs := GatherServiceAccounts(gatherer)()
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %#v", errs)
				return
			}
			bts, err := sa[0].Item.Marshal(context.Background())
			if err != nil {
				t.Fatalf("error marshalling %s", err)
			}
			s := string(bts)
			if test.exp != s {
				t.Fatalf("serviceaccount test failed. expected: %s got: %s", test.exp, s)
			}
		})
	}
}

func TestGatherStatefulSet(t *testing.T) {
	testSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "openshift-test",
		},
	}
	client := kubefake.NewSimpleClientset()
	_, err := client.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openshift-test"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake namespace", err)
	}
	_, err = client.AppsV1().StatefulSets("openshift-test").Create(context.Background(), &testSet, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake statefulset", err)
	}

	gatherer := &Gatherer{ctx: context.Background(), coreClient: client.CoreV1(), appsClient: client.AppsV1()}

	records, errs := GatherStatefulSets(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	item, err := records[0].Item.Marshal(context.TODO())
	var gatheredStatefulSet appsv1.StatefulSet
	_, _, err = appsV1Serializer.Decode(item, nil, &gatheredStatefulSet)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredStatefulSet.Name != "test-statefulset" {
		t.Fatalf("unexpected statefulset name %s", gatheredStatefulSet.Name)
	}

}

func TestGatherClusterOperator(t *testing.T) {
	testOperator := &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusteroperator",
		},
	}
	configCS := configfake.NewSimpleClientset()
	_, err := configCS.ConfigV1().ClusterOperators().Create(context.Background(), testOperator, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("unable to create fake clusteroperator", err)
	}
	gatherer := &Gatherer{ctx: context.Background(), client: configCS.ConfigV1(), discoveryClient: configCS.Discovery()}
	records, errs := GatherClusterOperators(gatherer)()
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	item, err := records[0].Item.Marshal(context.TODO())
	var gatheredCO configv1.ClusterOperator
	_, _, err = openshiftSerializer.Decode(item, nil, &gatheredCO)
	if err != nil {
		t.Fatalf("failed to decode object: %v", err)
	}
	if gatheredCO.Name != "test-clusteroperator" {
		t.Fatalf("unexpected clusteroperator name %s", gatheredCO.Name)
	}

}

func ExampleGatherMostRecentMetrics_Test() {
	b, err := ExampleMostRecentMetrics()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/metrics","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":"SGVsbG8sIGNsaWVudAojIEFMRVJUUyAyLzEwMDAKSGVsbG8sIGNsaWVudAo="}]
}

func ExampleGatherClusterOperators_Test() {
	b, err := ExampleClusterOperators()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/clusteroperator/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Degraded","status":"","lastTransitionTime":null}],"extension":null}}}]
}

func ExampleGatherNodes_Test() {
	b, err := ExampleNodes()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/node/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Ready","status":"False","lastHeartbeatTime":null,"lastTransitionTime":null}],"daemonEndpoints":{"kubeletEndpoint":{"Port":0}},"nodeInfo":{"machineID":"","systemUUID":"","bootID":"","kernelVersion":"","osImage":"","containerRuntimeVersion":"","kubeletVersion":"","kubeProxyVersion":"","operatingSystem":"","architecture":""}}}}]
}

func mustNotFail(t *testing.T, err interface{}, fmtstr string) {
	if e, ok := err.(error); ok && e != nil {
		t.Fatalf(fmtstr, e)
	}
	if e, ok := err.(bool); ok && !e {
		t.Fatalf(fmtstr, e)
	}
}

func findMap(cml *corev1.ConfigMapList, name string) *corev1.ConfigMap {
	for _, it := range cml.Items {
		if it.Name == name {
			return &it
		}
	}
	return nil
}
