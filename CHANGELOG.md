# Note: This CHANGELOG is only for the changes in insights operator. Please see OpenShift release notes for official changes
<!--Latest hash: e758cd083ea6758e3f94984323ba2d6e293f0db4-->
## 4.7

### Enhancements
- [#309](https://github.com/openshift/insights-operator/pull/309) Collect logs from openshift-sdn namespace
- [#273](https://github.com/openshift/insights-operator/pull/273) Implemented gathering specific logs from openshift apiserver operator
- [#297](https://github.com/openshift/insights-operator/pull/297) Gather netnamespaces network info
- [#314](https://github.com/openshift/insights-operator/pull/314) collect logs from openshift-sdn-controller pod
- [#291](https://github.com/openshift/insights-operator/pull/291) Gather SAP configuration (SCC & ClusterRoleBinding)
- [#316](https://github.com/openshift/insights-operator/pull/316) Gather list of OLM operator names and versions & minor clean up
- [#319](https://github.com/openshift/insights-operator/pull/319) Gather PersistentVolume definition (if any) used in Image registry st…

### Bug fixes
- [#317](https://github.com/openshift/insights-operator/pull/317) Update the sample archive and remove IP anonymization in clusteropera…
- [#320](https://github.com/openshift/insights-operator/pull/320) Monitors how many gatherings failed in a row, and applies degraded status accordingly

### Others
- [#298](https://github.com/openshift/insights-operator/pull/298) Bug 1908400:tests-e2e, increase timeouts, re-add TestArchiveUploadedAndResultsReceived
- [#295](https://github.com/openshift/insights-operator/pull/295) Skip TestArchiveUploadedAndResultReceived
- [#302](https://github.com/openshift/insights-operator/pull/302) Refactor periodic.go
- [#296](https://github.com/openshift/insights-operator/pull/296) e2e tests - increase timeouts little bit
- [#313](https://github.com/openshift/insights-operator/pull/313) Adds docs for using the profiler
- [#300](https://github.com/openshift/insights-operator/pull/300) Added changelog file
- [#279](https://github.com/openshift/insights-operator/pull/279) Refactoring record and gatherer
- [#310](https://github.com/openshift/insights-operator/pull/310) Remove HostSubnet anonymization

### Misc
- [#234](https://github.com/openshift/insights-operator/pull/234) Simplify/generalize host subnet pattern
- [#235](https://github.com/openshift/insights-operator/pull/235) add current profile annotations to CVO manifests
- [#216](https://github.com/openshift/insights-operator/pull/216) Adds ContainerRuntimeConfig gatherer
- [#281](https://github.com/openshift/insights-operator/pull/281) Fix bug in statefulset gatherer & minor doc fix
- [#237](https://github.com/openshift/insights-operator/pull/237) Add more verbosity to the tests
- [#292](https://github.com/openshift/insights-operator/pull/292) Update initial waiting times and give TestIsIOHealthy more time
- [#163](https://github.com/openshift/insights-operator/pull/163) Get report from smart-proxy and expose overview as a metric
- [#260](https://github.com/openshift/insights-operator/pull/260) Fix TestProxy in clusterauthorizer_test.go
- [#282](https://github.com/openshift/insights-operator/pull/282) Adds github pull request template.
- [#285](https://github.com/openshift/insights-operator/pull/285) Upgrade OpenShift & K8s API versions
- [#268](https://github.com/openshift/insights-operator/pull/268) fix typos in docs and unused variable
- [#274](https://github.com/openshift/insights-operator/pull/274) Add hostsubnet to sample archive & fix bug in the hostsu…
- [#209](https://github.com/openshift/insights-operator/pull/209) Add the namespace to the gatherers reports to avoid conflicts
- [#207](https://github.com/openshift/insights-operator/pull/207) Updating ose-insights-operator builder & base images to be consistent with ART
- [#236](https://github.com/openshift/insights-operator/pull/236) Refactor isOperatorDegraded and isOperatorDisabled to operatorConditionCheck
- [#220](https://github.com/openshift/insights-operator/pull/220) Updates the sample archive.
- [#246](https://github.com/openshift/insights-operator/pull/246) IO archive contains more records of than is the limit
- [#230](https://github.com/openshift/insights-operator/pull/230) Add IO Architecture doc and metrics sample
- [#272](https://github.com/openshift/insights-operator/pull/272) Fix clusteroperators serialization
- [#198](https://github.com/openshift/insights-operator/pull/198) IO becomes unhealthy due to a file change
- [#192](https://github.com/openshift/insights-operator/pull/192) Gather installplans
- [#265](https://github.com/openshift/insights-operator/pull/265) Skip TestArchiveUploadedAndResultReceived
- [#270](https://github.com/openshift/insights-operator/pull/270) First check IO container status and optionally delay first gathering
- [#225](https://github.com/openshift/insights-operator/pull/225) Fixes policyClient and the corresponding config.
- [#289](https://github.com/openshift/insights-operator/pull/289) Check context status when checking container is running OK
- [#174](https://github.com/openshift/insights-operator/pull/174) Improve container image collection
- [#196](https://github.com/openshift/insights-operator/pull/196) Add pattern/patterns to TestArchiveContains
- [#267](https://github.com/openshift/insights-operator/pull/267) Cleanup clusterOperatorInsights helper function
- [#212](https://github.com/openshift/insights-operator/pull/212) Fix error in default Smart proxy report endpoint
- [#249](https://github.com/openshift/insights-operator/pull/249) Update owners list
- [#275](https://github.com/openshift/insights-operator/pull/275) Adding a metrics report to IO gatherers
- [#241](https://github.com/openshift/insights-operator/pull/241) Added TestArchiveUploadedAndResultReceived
- [#200](https://github.com/openshift/insights-operator/pull/200) Gather ServiceAccounts stats from cluster namespaces
- [#142](https://github.com/openshift/insights-operator/pull/142) Report the returned response body to log the error detail from cloud.redhat.com
- [#211](https://github.com/openshift/insights-operator/pull/211) Take default support instead of rely on existence of config
- [#173](https://github.com/openshift/insights-operator/pull/173) Increase allowed delay in TestUploadNotDelayedAfterStart
- [#223](https://github.com/openshift/insights-operator/pull/223) Gather clusteroperator resources
- [#257](https://github.com/openshift/insights-operator/pull/257) Separating the gather logic into separate files
- [#312](https://github.com/openshift/insights-operator/pull/312) Updating ose-insights-operator builder & base images to be consistent with ART
- [#277](https://github.com/openshift/insights-operator/pull/277) Annotate manifests for single-node-developer cluster profile
- [#271](https://github.com/openshift/insights-operator/pull/271) Init health status metrics to distinguish no report state vs 0 problems
- [#259](https://github.com/openshift/insights-operator/pull/259) Add IBM Cloud managed annotations to CVO manifests
- [#210](https://github.com/openshift/insights-operator/pull/210) Gather MachineConfigPools
- [#264](https://github.com/openshift/insights-operator/pull/264) Reuse archives & refactor archive checks + some fixes
- [#218](https://github.com/openshift/insights-operator/pull/218) Gather StatefulSet configs from default & openshift namespaces
- [#266](https://github.com/openshift/insights-operator/pull/266) Collect complete spec info for cluster operator resources
- [#255](https://github.com/openshift/insights-operator/pull/255) Diskrecorder simplify the Summary function

## 4.6

### Misc
- [#119](https://github.com/openshift/insights-operator/pull/119) include node information in every archive
- [#148](https://github.com/openshift/insights-operator/pull/148) Limit collection of ALERTS metric to 1000 lines (~500KiB) to avoid unbearably large archives
- [#152](https://github.com/openshift/insights-operator/pull/152) Automate 2 BZ tests & generalize TestArchiveContainsFiles
- [#176](https://github.com/openshift/insights-operator/pull/176) rename operator container to be more descriptive
- [#155](https://github.com/openshift/insights-operator/pull/155) TestCSRCollected
- [#185](https://github.com/openshift/insights-operator/pull/185) Adds gatherer for PodDistributionBudget
- [#161](https://github.com/openshift/insights-operator/pull/161) Updating archive and Generated doc
- [#157](https://github.com/openshift/insights-operator/pull/157) TestUploadNotDelayedAfterStart
- [#133](https://github.com/openshift/insights-operator/pull/133) Running Red Hat images and crashlooping OpenShift pods should be gathered
- [#165](https://github.com/openshift/insights-operator/pull/165) Log checker
- [#177](https://github.com/openshift/insights-operator/pull/177) Collect hostsubnet information
- [#187](https://github.com/openshift/insights-operator/pull/187) Add new team members to OWNERS
- [#183](https://github.com/openshift/insights-operator/pull/183) Gather MachineSet info
- [#184](https://github.com/openshift/insights-operator/pull/184) Limit the maximum number of CSR
- [#126](https://github.com/openshift/insights-operator/pull/126) limit the size of collected logs
- [#150](https://github.com/openshift/insights-operator/pull/150) Test if files in insights archive have extension set
- [#132](https://github.com/openshift/insights-operator/pull/132) Check also Pod status before enabling Fast upload
- [#124](https://github.com/openshift/insights-operator/pull/124) Make e2e tests more stable
- [#168](https://github.com/openshift/insights-operator/pull/168) handle 201 response from upload
- [#149](https://github.com/openshift/insights-operator/pull/149) TestCollectingAfterDegradingOperator
- [#178](https://github.com/openshift/insights-operator/pull/178) Updates readme
- [#171](https://github.com/openshift/insights-operator/pull/171) Add metrics back to archive sample
- [#134](https://github.com/openshift/insights-operator/pull/134) Test pods logs collected - Automate BZ1838973
- [#135](https://github.com/openshift/insights-operator/pull/135) Shorten e2e tests interval
- [#129](https://github.com/openshift/insights-operator/pull/129) Updating sample data
- [#197](https://github.com/openshift/insights-operator/pull/197) Adds info about sample archive in README.md
- [#179](https://github.com/openshift/insights-operator/pull/179) Updating Dockerfile baseimages to mach ocp-build-data config
- [#167](https://github.com/openshift/insights-operator/pull/167) Updating Dockerfile baseimages to mach ocp-build-data config
- [#125](https://github.com/openshift/insights-operator/pull/125) Collect namespace level cpu and memory metrics
- [#158](https://github.com/openshift/insights-operator/pull/158) Decrease insights secret interval minimal duration
- [#188](https://github.com/openshift/insights-operator/pull/188) Do not return CRD not found error, just log it
- [#115](https://github.com/openshift/insights-operator/pull/115) store pod logs
- [#154](https://github.com/openshift/insights-operator/pull/154) Add @natiiix to OWNERS
- [#166](https://github.com/openshift/insights-operator/pull/166) Gather VolumeSnapshot CRD
- [#182](https://github.com/openshift/insights-operator/pull/182) Automate TestArchiveContains::HostsSubnet & 2 more
- [#186](https://github.com/openshift/insights-operator/pull/186) Upgrade to k8s 0.18.9
- [#175](https://github.com/openshift/insights-operator/pull/175) Adds cluster version into the User-Agent header
- [#144](https://github.com/openshift/insights-operator/pull/144) TestPodLogsCollected fix
- [#141](https://github.com/openshift/insights-operator/pull/141) Info how to retrieve key and certificate and simple script to do so
- [#159](https://github.com/openshift/insights-operator/pull/159) Check if insights operator records an event
- [#193](https://github.com/openshift/insights-operator/pull/193) Make gen-doc work outside of GOPATH
- [#114](https://github.com/openshift/insights-operator/pull/114) Set reasons for conditions

## 4.5

### Misc
- [#90](https://github.com/openshift/insights-operator/pull/90) Specify bugzilla component in OWNERS
- [#88](https://github.com/openshift/insights-operator/pull/88) Report logs when checkPods is going to fail
- [#95](https://github.com/openshift/insights-operator/pull/95) Refactoring collector, add Doc and doc generator
- [#77](https://github.com/openshift/insights-operator/pull/77) Insightsclient metrics - small bugfix , added status code '0'.
- [#81](https://github.com/openshift/insights-operator/pull/81) add new test TestClusterDefaultNodeSelector
- [#102](https://github.com/openshift/insights-operator/pull/102) Stop using service ca from service account token
- [#100](https://github.com/openshift/insights-operator/pull/100) Gather image registry config
- [#113](https://github.com/openshift/insights-operator/pull/113) Gathering Image Pruner configuration
- [#78](https://github.com/openshift/insights-operator/pull/78) Insights operator does not require being in an openshift run-level to function
- [#84](https://github.com/openshift/insights-operator/pull/84) Fix reporting duration error and add tests
- [#99](https://github.com/openshift/insights-operator/pull/99) add json extension 
- [#73](https://github.com/openshift/insights-operator/pull/73) Add license
- [#94](https://github.com/openshift/insights-operator/pull/94) add Martin Kunc to OWNERS
- [#117](https://github.com/openshift/insights-operator/pull/117) Skip the initial upload delay
- [#86](https://github.com/openshift/insights-operator/pull/86) Collecting config maps
- [#85](https://github.com/openshift/insights-operator/pull/85) Add test to observe config changes
- [#70](https://github.com/openshift/insights-operator/pull/70) Collect certificates
- [#72](https://github.com/openshift/insights-operator/pull/72) Updated base image for insights-operator
- [#87](https://github.com/openshift/insights-operator/pull/87) Support for specific http proxy for the service
- [#93](https://github.com/openshift/insights-operator/pull/93) Increase tests timeout and ignore failing tests
- [#82](https://github.com/openshift/insights-operator/pull/82) add coverage for BZ1753755

## 4.4

### Misc
- [#71](https://github.com/openshift/insights-operator/pull/71) Add alexandrevicenzi as code owner
- [#69](https://github.com/openshift/insights-operator/pull/69) Only return pods that have been pending more than 2m
- [#65](https://github.com/openshift/insights-operator/pull/65) added TestUnreachableHost
- [#68](https://github.com/openshift/insights-operator/pull/68) Update insights-operator to latest library-go

