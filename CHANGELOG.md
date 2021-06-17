# Note: This CHANGELOG is only for the changes in insights operator. 
	Please see OpenShift release notes for official changes\n<!--Latest hash: 5dcf37aef015ce79319468000ef178ad97f29416-->
## 4.8

### Enhancement
- [#438](https://github.com/openshift/insights-operator/pull/438) Gather MachineAutoscalers definitions
- [#442](https://github.com/openshift/insights-operator/pull/442) include full timestamps in the logs
- [#432](https://github.com/openshift/insights-operator/pull/432) Replace gather-job image without FQDN
- [#431](https://github.com/openshift/insights-operator/pull/431) Change event gathering interval
- [#421](https://github.com/openshift/insights-operator/pull/421) Collect full pod log for stack traces
- [#422](https://github.com/openshift/insights-operator/pull/422) Gather SDI-related MachineConfigs
- [#429](https://github.com/openshift/insights-operator/pull/429) Adding GatherMachineHealthCheck
- [#426](https://github.com/openshift/insights-operator/pull/426) breaking changes in pr template
- [#427](https://github.com/openshift/insights-operator/pull/427) Adds virt_platform metric to the collected metrics
- [#399](https://github.com/openshift/insights-operator/pull/399) Support of gatherers with different periods
- [#414](https://github.com/openshift/insights-operator/pull/414) Add vsphere_node_hw_version_total metric to the collected metrics
- [#405](https://github.com/openshift/insights-operator/pull/405) Rename workload annotations
- [#374](https://github.com/openshift/insights-operator/pull/374) Gather summary of PodNetworkConnectivityChecks
- [#397](https://github.com/openshift/insights-operator/pull/397) Split up the GatherClusterOperators into smaller parts
- [#400](https://github.com/openshift/insights-operator/pull/400) Extend OLM data with CSV display name
- [#391](https://github.com/openshift/insights-operator/pull/391) Add management workload annotations
- [#315](https://github.com/openshift/insights-operator/pull/315) Add a workload fingerprint gatherer
- [#354](https://github.com/openshift/insights-operator/pull/354) Obfuscate IPv4 addresses and hide cluster base domain
- [#344](https://github.com/openshift/insights-operator/pull/344) dockerfile for remote debugging
- [#355](https://github.com/openshift/insights-operator/pull/355) Gather related pod logs when a cluster operator is degraded
- [#376](https://github.com/openshift/insights-operator/pull/376) Gahter datahubs.installers.datahub.sap.com resources from SAP clusters
- [#356](https://github.com/openshift/insights-operator/pull/356) Adds memory usage to the metadata
- [#358](https://github.com/openshift/insights-operator/pull/358)  Extend the OLM operator data with related ClusterServiceVersion conditions
- [#347](https://github.com/openshift/insights-operator/pull/347) Gather info about unhealthy SAP pods
- [#342](https://github.com/openshift/insights-operator/pull/342) sap license management logs gatherer
- [#337](https://github.com/openshift/insights-operator/pull/337) Recorder refactoring that improves maintainability
- [#341](https://github.com/openshift/insights-operator/pull/341) Fixes changelog script code styling
- [#303](https://github.com/openshift/insights-operator/pull/303) Improve code removing some codesmells

### Bugfix
- [#445](https://github.com/openshift/insights-operator/pull/445) Fixes one small bug
- [#425](https://github.com/openshift/insights-operator/pull/425) Do not exceed archive size limit
- [#424](https://github.com/openshift/insights-operator/pull/424) fixed obfuscation permissions
- [#418](https://github.com/openshift/insights-operator/pull/418) #417 insights report - add basic retry logic in case of 404
- [#412](https://github.com/openshift/insights-operator/pull/412) Remove URL anonymization from ClusterOperator resources
- [#408](https://github.com/openshift/insights-operator/pull/408) Add missing sample archive data
- [#406](https://github.com/openshift/insights-operator/pull/406) DelegatingAuthenticationOptions TokenReview request timeout
- [#404](https://github.com/openshift/insights-operator/pull/404) Make the pods limit in the workload gatherer more accurate
- [#401](https://github.com/openshift/insights-operator/pull/401) Update configmap gatherer to not fail in case of invalid yaml
- [#386](https://github.com/openshift/insights-operator/pull/386) Remove some unnecessary obfuscation
- [#368](https://github.com/openshift/insights-operator/pull/368) Include namespace name in binarydata configmap path & test
- [#365](https://github.com/openshift/insights-operator/pull/365) Do not scan all the pod logs in the "GatherOpenshiftAuthenticationLogs"
- [#352](https://github.com/openshift/insights-operator/pull/352) Do not use context in the recorder
- [#336](https://github.com/openshift/insights-operator/pull/336) Disable instead of Degrade in case of gather fails
- [#334](https://github.com/openshift/insights-operator/pull/334) Do not create the metrics file in case of any error
- [#332](https://github.com/openshift/insights-operator/pull/332) Relax the recent log gatherers to avoid degrading during…
- [#329](https://github.com/openshift/insights-operator/pull/329) Remove StatefulSet gatherer & replace it with gathering "cluster-mon…

### Others
- [#439](https://github.com/openshift/insights-operator/pull/439) Adds tasks pool to tasks_processing
- [#441](https://github.com/openshift/insights-operator/pull/441) Use configured interval as the event time limit & check series if
- [#436](https://github.com/openshift/insights-operator/pull/436) Adds more tests for periodic.go
- [#448](https://github.com/openshift/insights-operator/pull/448) Replace golint with revive
- [#419](https://github.com/openshift/insights-operator/pull/419) Store translation table in a secret
- [#443](https://github.com/openshift/insights-operator/pull/443) Fixes the remaining lint issues
- [#440](https://github.com/openshift/insights-operator/pull/440) Workloads gatherer - increase the pods limit
- [#437](https://github.com/openshift/insights-operator/pull/437) Update K8s & OpenShift API versions
- [#430](https://github.com/openshift/insights-operator/pull/430) Fixes gendoc
- [#415](https://github.com/openshift/insights-operator/pull/415) Fix pre-commit script for staged vendor files
- [#409](https://github.com/openshift/insights-operator/pull/409) Add a few tests to configobserver_test.go
- [#420](https://github.com/openshift/insights-operator/pull/420) Improves documentation of GatherClusterOperatorPodsAndEvents
- [#407](https://github.com/openshift/insights-operator/pull/407) Linting fixes in gather package
- [#398](https://github.com/openshift/insights-operator/pull/398) Docs and lint fixes
- [#395](https://github.com/openshift/insights-operator/pull/395) style fixes by GoLand and golangci-lint
- [#396](https://github.com/openshift/insights-operator/pull/396) Workloads - Add limit for the number of pods gathered
- [#389](https://github.com/openshift/insights-operator/pull/389) One-off gather
- [#392](https://github.com/openshift/insights-operator/pull/392) Disable emptyStringTest check
- [#390](https://github.com/openshift/insights-operator/pull/390) Adding githooks, contributing and styleguide
- [#388](https://github.com/openshift/insights-operator/pull/388) Adding CI Liting and improving Makefile
- [#387](https://github.com/openshift/insights-operator/pull/387) Integration tests moved to internal Python repo
- [#385](https://github.com/openshift/insights-operator/pull/385) Add OCP versions to particular gatherers
- [#377](https://github.com/openshift/insights-operator/pull/377) Fixing code style
- [#371](https://github.com/openshift/insights-operator/pull/371) Introduce quick gather command
- [#359](https://github.com/openshift/insights-operator/pull/359) Update documentation
- [#357](https://github.com/openshift/insights-operator/pull/357) Makes changelog script compatible with squash
- [#353](https://github.com/openshift/insights-operator/pull/353) Update relatedObjects
- [#351](https://github.com/openshift/insights-operator/pull/351) Reduce Gatherer's code complexity
- [#350](https://github.com/openshift/insights-operator/pull/350) Remove code duplication that disable the gather
- [#348](https://github.com/openshift/insights-operator/pull/348) Do not run gathering when IO is disabled
- [#349](https://github.com/openshift/insights-operator/pull/349) Sample archive - update metrics file to contain all the metrics we ga…
- [#345](https://github.com/openshift/insights-operator/pull/345) Small clean up and utils reorg
- [#306](https://github.com/openshift/insights-operator/pull/306) Introduce parallelism to unit tests
- [#305](https://github.com/openshift/insights-operator/pull/305) Some charms to Makefile
- [#318](https://github.com/openshift/insights-operator/pull/318) Auto changelog

### Misc
- [#380](https://github.com/openshift/insights-operator/pull/380) Updating ose-insights-operator builder & base images to be consistent with ART
- [#381](https://github.com/openshift/insights-operator/pull/381) Gather openshift-cluster-version pods and events
- [#333](https://github.com/openshift/insights-operator/pull/333) Updating ose-insights-operator builder & base images to be consistent with ART

## 4.7

### Enhancement
- [#327](https://github.com/openshift/insights-operator/pull/327) collect invalid resource name error from logs 
- [#316](https://github.com/openshift/insights-operator/pull/316) Gather list of OLM operator names and versions & minor clean up
- [#319](https://github.com/openshift/insights-operator/pull/319) Gather PersistentVolume definition (if any) used in Image registry st…
- [#291](https://github.com/openshift/insights-operator/pull/291) Gather SAP configuration (SCC & ClusterRoleBinding)
- [#314](https://github.com/openshift/insights-operator/pull/314) collect logs from openshift-sdn-controller pod
- [#309](https://github.com/openshift/insights-operator/pull/309) Collect logs from openshift-sdn namespace
- [#273](https://github.com/openshift/insights-operator/pull/273) Implemented gathering specific logs from openshift apiserver operator
- [#297](https://github.com/openshift/insights-operator/pull/297) Gather netnamespaces network info

### Bugfix
- [#325](https://github.com/openshift/insights-operator/pull/325) Fixes error metadata gathering
- [#320](https://github.com/openshift/insights-operator/pull/320) Monitors how many gatherings failed in a row, and applies degraded status accordingly
- [#317](https://github.com/openshift/insights-operator/pull/317) Update the sample archive and remove IP anonymization in clusteropera…

### Others
- [#323](https://github.com/openshift/insights-operator/pull/323) Updates arch.md
- [#302](https://github.com/openshift/insights-operator/pull/302) Refactor periodic.go
- [#313](https://github.com/openshift/insights-operator/pull/313) Adds docs for using the profiler
- [#310](https://github.com/openshift/insights-operator/pull/310) Remove HostSubnet anonymization
- [#300](https://github.com/openshift/insights-operator/pull/300) Added changelog file
- [#298](https://github.com/openshift/insights-operator/pull/298) Bug 1908400:tests-e2e, increase timeouts, re-add TestArchiveUploadedAndResultsReceived
- [#279](https://github.com/openshift/insights-operator/pull/279) Refactoring record and gatherer
- [#296](https://github.com/openshift/insights-operator/pull/296) e2e tests - increase timeouts little bit
- [#295](https://github.com/openshift/insights-operator/pull/295) Skip TestArchiveUploadedAndResultReceived

### Misc
- [#312](https://github.com/openshift/insights-operator/pull/312) Updating ose-insights-operator builder & base images to be consistent with ART
- [#285](https://github.com/openshift/insights-operator/pull/285) Upgrade OpenShift & K8s API versions
- [#282](https://github.com/openshift/insights-operator/pull/282) Adds github pull request template.
- [#255](https://github.com/openshift/insights-operator/pull/255) Diskrecorder simplify the Summary function
- [#292](https://github.com/openshift/insights-operator/pull/292) Update initial waiting times and give TestIsIOHealthy more time
- [#289](https://github.com/openshift/insights-operator/pull/289) Check context status when checking container is running OK
- [#275](https://github.com/openshift/insights-operator/pull/275) Adding a metrics report to IO gatherers
- [#270](https://github.com/openshift/insights-operator/pull/270) First check IO container status and optionally delay first gathering
- [#281](https://github.com/openshift/insights-operator/pull/281) Fix bug in statefulset gatherer & minor doc fix
- [#267](https://github.com/openshift/insights-operator/pull/267) Cleanup clusterOperatorInsights helper function
- [#277](https://github.com/openshift/insights-operator/pull/277) Annotate manifests for single-node-developer cluster profile
- [#266](https://github.com/openshift/insights-operator/pull/266) Collect complete spec info for cluster operator resources
- [#274](https://github.com/openshift/insights-operator/pull/274) Add hostsubnet to sample archive & fix bug in the hostsu…
- [#264](https://github.com/openshift/insights-operator/pull/264) Reuse archives & refactor archive checks + some fixes
- [#272](https://github.com/openshift/insights-operator/pull/272) Fix clusteroperators serialization
- [#271](https://github.com/openshift/insights-operator/pull/271) Init health status metrics to distinguish no report state vs 0 problems
- [#268](https://github.com/openshift/insights-operator/pull/268) fix typos in docs and unused variable
- [#174](https://github.com/openshift/insights-operator/pull/174) Improve container image collection
- [#230](https://github.com/openshift/insights-operator/pull/230) Add IO Architecture doc and metrics sample
- [#265](https://github.com/openshift/insights-operator/pull/265) Skip TestArchiveUploadedAndResultReceived
- [#257](https://github.com/openshift/insights-operator/pull/257) Separating the gather logic into separate files
- [#259](https://github.com/openshift/insights-operator/pull/259) Add IBM Cloud managed annotations to CVO manifests
- [#260](https://github.com/openshift/insights-operator/pull/260) Fix TestProxy in clusterauthorizer_test.go
- [#249](https://github.com/openshift/insights-operator/pull/249) Update owners list
- [#236](https://github.com/openshift/insights-operator/pull/236) Refactor isOperatorDegraded and isOperatorDisabled to operatorConditionCheck
- [#196](https://github.com/openshift/insights-operator/pull/196) Add pattern/patterns to TestArchiveContains
- [#246](https://github.com/openshift/insights-operator/pull/246) IO archive contains more records of than is the limit
- [#223](https://github.com/openshift/insights-operator/pull/223) Gather clusteroperator resources
- [#235](https://github.com/openshift/insights-operator/pull/235) add current profile annotations to CVO manifests
- [#241](https://github.com/openshift/insights-operator/pull/241) Added TestArchiveUploadedAndResultReceived
- [#234](https://github.com/openshift/insights-operator/pull/234) Simplify/generalize host subnet pattern
- [#237](https://github.com/openshift/insights-operator/pull/237) Add more verbosity to the tests
- [#218](https://github.com/openshift/insights-operator/pull/218) Gather StatefulSet configs from default & openshift namespaces
- [#220](https://github.com/openshift/insights-operator/pull/220) Updates the sample archive.
- [#225](https://github.com/openshift/insights-operator/pull/225) Fixes policyClient and the corresponding config.
- [#173](https://github.com/openshift/insights-operator/pull/173) Increase allowed delay in TestUploadNotDelayedAfterStart
- [#192](https://github.com/openshift/insights-operator/pull/192) Gather installplans
- [#216](https://github.com/openshift/insights-operator/pull/216) Adds ContainerRuntimeConfig gatherer
- [#212](https://github.com/openshift/insights-operator/pull/212) Fix error in default Smart proxy report endpoint
- [#211](https://github.com/openshift/insights-operator/pull/211) Take default support instead of rely on existence of config
- [#163](https://github.com/openshift/insights-operator/pull/163) Get report from smart-proxy and expose overview as a metric
- [#207](https://github.com/openshift/insights-operator/pull/207) Updating ose-insights-operator builder & base images to be consistent with ART
- [#210](https://github.com/openshift/insights-operator/pull/210) Gather MachineConfigPools
- [#209](https://github.com/openshift/insights-operator/pull/209) Add the namespace to the gatherers reports to avoid conflicts
- [#142](https://github.com/openshift/insights-operator/pull/142) Report the returned response body to log the error detail from cloud.redhat.com
- [#198](https://github.com/openshift/insights-operator/pull/198) IO becomes unhealthy due to a file change
- [#200](https://github.com/openshift/insights-operator/pull/200) Gather ServiceAccounts stats from cluster namespaces

## 4.6

### Misc
- [#197](https://github.com/openshift/insights-operator/pull/197) Adds info about sample archive in README.md
- [#185](https://github.com/openshift/insights-operator/pull/185) Adds gatherer for PodDistributionBudget
- [#184](https://github.com/openshift/insights-operator/pull/184) Limit the maximum number of CSR
- [#175](https://github.com/openshift/insights-operator/pull/175) Adds cluster version into the User-Agent header
- [#165](https://github.com/openshift/insights-operator/pull/165) Log checker
- [#182](https://github.com/openshift/insights-operator/pull/182) Automate TestArchiveContains::HostsSubnet & 2 more
- [#178](https://github.com/openshift/insights-operator/pull/178) Updates readme
- [#193](https://github.com/openshift/insights-operator/pull/193) Make gen-doc work outside of GOPATH
- [#186](https://github.com/openshift/insights-operator/pull/186) Upgrade to k8s 0.18.9
- [#183](https://github.com/openshift/insights-operator/pull/183) Gather MachineSet info
- [#187](https://github.com/openshift/insights-operator/pull/187) Add new team members to OWNERS
- [#188](https://github.com/openshift/insights-operator/pull/188) Do not return CRD not found error, just log it
- [#179](https://github.com/openshift/insights-operator/pull/179) Updating Dockerfile baseimages to mach ocp-build-data config
- [#177](https://github.com/openshift/insights-operator/pull/177) Collect hostsubnet information
- [#171](https://github.com/openshift/insights-operator/pull/171) Add metrics back to archive sample
- [#166](https://github.com/openshift/insights-operator/pull/166) Gather VolumeSnapshot CRD
- [#176](https://github.com/openshift/insights-operator/pull/176) rename operator container to be more descriptive
- [#167](https://github.com/openshift/insights-operator/pull/167) Updating Dockerfile baseimages to mach ocp-build-data config
- [#168](https://github.com/openshift/insights-operator/pull/168) handle 201 response from upload
- [#161](https://github.com/openshift/insights-operator/pull/161) Updating archive and Generated doc
- [#159](https://github.com/openshift/insights-operator/pull/159) Check if insights operator records an event
- [#157](https://github.com/openshift/insights-operator/pull/157) TestUploadNotDelayedAfterStart
- [#158](https://github.com/openshift/insights-operator/pull/158) Decrease insights secret interval minimal duration
- [#155](https://github.com/openshift/insights-operator/pull/155) TestCSRCollected
- [#154](https://github.com/openshift/insights-operator/pull/154) Add @natiiix to OWNERS
- [#152](https://github.com/openshift/insights-operator/pull/152) Automate 2 BZ tests & generalize TestArchiveContainsFiles
- [#148](https://github.com/openshift/insights-operator/pull/148) Limit collection of ALERTS metric to 1000 lines (~500KiB) to avoid unbearably large archives
- [#150](https://github.com/openshift/insights-operator/pull/150) Test if files in insights archive have extension set
- [#149](https://github.com/openshift/insights-operator/pull/149) TestCollectingAfterDegradingOperator
- [#133](https://github.com/openshift/insights-operator/pull/133) Running Red Hat images and crashlooping OpenShift pods should be gathered
- [#135](https://github.com/openshift/insights-operator/pull/135) Shorten e2e tests interval
- [#144](https://github.com/openshift/insights-operator/pull/144) TestPodLogsCollected fix
- [#134](https://github.com/openshift/insights-operator/pull/134) Test pods logs collected - Automate BZ1838973
- [#141](https://github.com/openshift/insights-operator/pull/141) Info how to retrieve key and certificate and simple script to do so
- [#132](https://github.com/openshift/insights-operator/pull/132) Check also Pod status before enabling Fast upload
- [#129](https://github.com/openshift/insights-operator/pull/129) Updating sample data
- [#126](https://github.com/openshift/insights-operator/pull/126) limit the size of collected logs
- [#119](https://github.com/openshift/insights-operator/pull/119) include node information in every archive
- [#125](https://github.com/openshift/insights-operator/pull/125) Collect namespace level cpu and memory metrics
- [#124](https://github.com/openshift/insights-operator/pull/124) Make e2e tests more stable
- [#115](https://github.com/openshift/insights-operator/pull/115) store pod logs
- [#114](https://github.com/openshift/insights-operator/pull/114) Set reasons for conditions

## 4.5

### Misc
- [#117](https://github.com/openshift/insights-operator/pull/117) Skip the initial upload delay
- [#99](https://github.com/openshift/insights-operator/pull/99) add json extension 
- [#113](https://github.com/openshift/insights-operator/pull/113) Gathering Image Pruner configuration
- [#102](https://github.com/openshift/insights-operator/pull/102) Stop using service ca from service account token
- [#100](https://github.com/openshift/insights-operator/pull/100) Gather image registry config
- [#95](https://github.com/openshift/insights-operator/pull/95) Refactoring collector, add Doc and doc generator
- [#94](https://github.com/openshift/insights-operator/pull/94) add Martin Kunc to OWNERS
- [#93](https://github.com/openshift/insights-operator/pull/93) Increase tests timeout and ignore failing tests
- [#86](https://github.com/openshift/insights-operator/pull/86) Collecting config maps
- [#90](https://github.com/openshift/insights-operator/pull/90) Specify bugzilla component in OWNERS
- [#87](https://github.com/openshift/insights-operator/pull/87) Support for specific http proxy for the service
- [#88](https://github.com/openshift/insights-operator/pull/88) Report logs when checkPods is going to fail
- [#85](https://github.com/openshift/insights-operator/pull/85) Add test to observe config changes
- [#84](https://github.com/openshift/insights-operator/pull/84) Fix reporting duration error and add tests
- [#82](https://github.com/openshift/insights-operator/pull/82) add coverage for BZ1753755
- [#81](https://github.com/openshift/insights-operator/pull/81) add new test TestClusterDefaultNodeSelector
- [#78](https://github.com/openshift/insights-operator/pull/78) Insights operator does not require being in an openshift run-level to function
- [#72](https://github.com/openshift/insights-operator/pull/72) Updated base image for insights-operator
- [#70](https://github.com/openshift/insights-operator/pull/70) Collect certificates
- [#73](https://github.com/openshift/insights-operator/pull/73) Add license
- [#77](https://github.com/openshift/insights-operator/pull/77) Insightsclient metrics - small bugfix , added status code '0'.

## 4.4

### Misc
- [#71](https://github.com/openshift/insights-operator/pull/71) Add alexandrevicenzi as code owner
- [#65](https://github.com/openshift/insights-operator/pull/65) added TestUnreachableHost
- [#68](https://github.com/openshift/insights-operator/pull/68) Update insights-operator to latest library-go
- [#69](https://github.com/openshift/insights-operator/pull/69) Only return pods that have been pending more than 2m
- [#66](https://github.com/openshift/insights-operator/pull/66) include error message when we are unable to build request
- [#62](https://github.com/openshift/insights-operator/pull/62) Add Pavel Tisnovsky into list of repo owners
- [#61](https://github.com/openshift/insights-operator/pull/61) added TestOptOutOptIn and moved some code to functions
- [#59](https://github.com/openshift/insights-operator/pull/59) Bug 1782151 - override node selector

## 

### Enhancement
- [#446](https://github.com/openshift/insights-operator/pull/446) add egress ips support to anonymizer

### Bugfix
- [#444](https://github.com/openshift/insights-operator/pull/444) MemoryRecord name can be obfuscated & fix case of duplicate records

### Others
- [#452](https://github.com/openshift/insights-operator/pull/452) Update versions in the metrics gather documentation

