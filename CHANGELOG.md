# Note: This CHANGELOG is only for the changes in insights operator.
	Please see OpenShift release notes for official changes\n<!--Latest hash: 76cc46b739c27281db0f7b2a23eb091b2a70f698-->
## 4.13

### Data Enhancement
- [#741](https://github.com/openshift/insights-operator/pull/685) Gather openshift-monitoring persistent volumes

### Bugfix
- [#723](https://github.com/openshift/insights-operator/pull/723) Obfuscate HTTP_PROXY and HTTPS_PROXY env variables on containers

### Others
- [#693](https://github.com/openshift/insights-operator/pull/693) Use cgroups memory usage data in the archive metadata
- [#733](https://github.com/openshift/insights-operator/pull/733) Renamed log gatherer file (SDN controller) and unit tests

## 4.12

### Data Enhancement
- [#685](https://github.com/openshift/insights-operator/pull/685) remove name and namespace from dvo metrics
- [#658](https://github.com/openshift/insights-operator/pull/658) openshift-machine-api warning events gatherer
- [#646](https://github.com/openshift/insights-operator/pull/646) adding insights capability annotations
- [#657](https://github.com/openshift/insights-operator/pull/657) helm upgrade and uninstall metric gathering
- [#654](https://github.com/openshift/insights-operator/pull/654) Gather status of the cephclusters.ceph.rook.io resources
- [#652](https://github.com/openshift/insights-operator/pull/652) Gather & store firing alerts in JSON too

### Bugfix
- [#683](https://github.com/openshift/insights-operator/pull/683) Updated info link in insights recommendations
- [#687](https://github.com/openshift/insights-operator/pull/687) fix the schema checking conditional gathering rules
- [#681](https://github.com/openshift/insights-operator/pull/681) limit the size of logs loaded into memory
- [#679](https://github.com/openshift/insights-operator/pull/679) Update PNCC gatherer
- [#678](https://github.com/openshift/insights-operator/pull/678) do not include disabled rules in the total metric
- [#670](https://github.com/openshift/insights-operator/pull/670) updated conditional gathering rules checking
- [#674](https://github.com/openshift/insights-operator/pull/674) fix alert namespace label
- [#672](https://github.com/openshift/insights-operator/pull/672) Explicitly clear run-level label
- [#664](https://github.com/openshift/insights-operator/pull/664) update the DVO metrics gatherer
- [#667](https://github.com/openshift/insights-operator/pull/667) order conditions by type to limit un-needed updates

### Others
- [#650](https://github.com/openshift/insights-operator/pull/650) reduce cognitive complexity
- [#690](https://github.com/openshift/insights-operator/pull/690) Improve GatherNodeLogs docs
- [#688](https://github.com/openshift/insights-operator/pull/688) Update owners list
- [#680](https://github.com/openshift/insights-operator/pull/680) read DataPolicy attribute from the config API
- [#673](https://github.com/openshift/insights-operator/pull/673) read new config API and disable gatherers based on the API values
- [#669](https://github.com/openshift/insights-operator/pull/669) Implement insights report updating in the insightsoperators.operator.openshift.io resource
- [#671](https://github.com/openshift/insights-operator/pull/671) K8s & OpenShift version updates
- [#666](https://github.com/openshift/insights-operator/pull/666) Introduce insightsoperators.openshift.io CR & implement its gather st…
- [#661](https://github.com/openshift/insights-operator/pull/661) Update K8s & OpenShift versions + vendoring
- [#660](https://github.com/openshift/insights-operator/pull/660) Remove Bugzilla references
- [#656](https://github.com/openshift/insights-operator/pull/656) Extend the conditional gatherer docs
- [#653](https://github.com/openshift/insights-operator/pull/653) Enable Insights recommendations as alerts by default
- [#644](https://github.com/openshift/insights-operator/pull/644) Expose Insights recommendations as alerts
- [#647](https://github.com/openshift/insights-operator/pull/647) Minor gatherer's docs & OWNERS update
- [#645](https://github.com/openshift/insights-operator/pull/645) adding list of insights generated metrics

### Misc
- [#682](https://github.com/openshift/insights-operator/pull/682) Updating ose-insights-operator images to be consistent with ART
- [#649](https://github.com/openshift/insights-operator/pull/649) Updating ose-insights-operator images to be consistent with ART

## 4.11

### Data Enhancement
- [#625](https://github.com/openshift/insights-operator/pull/625) gather io configuration
- [#627](https://github.com/openshift/insights-operator/pull/627) Console helm metrics
- [#603](https://github.com/openshift/insights-operator/pull/603) Implement fingerprint for records
- [#614](https://github.com/openshift/insights-operator/pull/614) Gather ODF config data
- [#604](https://github.com/openshift/insights-operator/pull/604) Gather namespace names with overlapping UIDs
- [#596](https://github.com/openshift/insights-operator/pull/596) Gather some error messages from the kube-controller-manager containers
- [#576](https://github.com/openshift/insights-operator/pull/576) pod_definition conditional gather
- [#579](https://github.com/openshift/insights-operator/pull/579) collecting logs if certain alerts are raised
- [#580](https://github.com/openshift/insights-operator/pull/580) Gather cluster images.config.openshift.io resource definition

### Bugfix
- [#641](https://github.com/openshift/insights-operator/pull/641) insightsclient - do not format OCM error message twice
- [#640](https://github.com/openshift/insights-operator/pull/640) Fix permissions for OCS for the storage gatherer
- [#633](https://github.com/openshift/insights-operator/pull/633) make cluster version condition more flexible
- [#620](https://github.com/openshift/insights-operator/pull/620) save conditional gatherer endpoint and firing alerts in the metadata
- [#618](https://github.com/openshift/insights-operator/pull/618) Fix the clusteroperator conditions values when IO is
- [#613](https://github.com/openshift/insights-operator/pull/613) Fix vendoring of the build-machinery-go
- [#601](https://github.com/openshift/insights-operator/pull/601) save version of gathering rules in metadata
- [#595](https://github.com/openshift/insights-operator/pull/595) Set default messages & reconcile clusteroperator status conditions
- [#589](https://github.com/openshift/insights-operator/pull/589) Don't serialize empty `images` attribute in the workload info gatherer
- [#584](https://github.com/openshift/insights-operator/pull/584) Set default messages & reconcile clusteroperator status conditions
- [#584](https://github.com/openshift/insights-operator/pull/584) Set default messages & reconcile clusteroperator status conditions
- [#578](https://github.com/openshift/insights-operator/pull/578) defer in loop

### Others
- [#642](https://github.com/openshift/insights-operator/pull/642) Update CHANGELOG
- [#639](https://github.com/openshift/insights-operator/pull/639) Do not use the kube-rbac-proxy container
- [#637](https://github.com/openshift/insights-operator/pull/637) Implement Prometheus Collector pattern
- [#626](https://github.com/openshift/insights-operator/pull/626) update of the arch.md document
- [#621](https://github.com/openshift/insights-operator/pull/621) create new permanent clusteroperator conditions for SCA &
- [#607](https://github.com/openshift/insights-operator/pull/607) Implement Prometheus Collector pattern
- [#629](https://github.com/openshift/insights-operator/pull/629) bump(k8s v0.24.0)
- [#631](https://github.com/openshift/insights-operator/pull/631) Update links to machine-api types
- [#622](https://github.com/openshift/insights-operator/pull/622) Update to console.redhat.com services
- [#617](https://github.com/openshift/insights-operator/pull/617) Update new gatherer OCP versions
- [#571](https://github.com/openshift/insights-operator/pull/571) Cluster transfer OCM controller
- [#606](https://github.com/openshift/insights-operator/pull/606) Minor gatherer documentation update
- [#600](https://github.com/openshift/insights-operator/pull/600) Create a new Prometheus metric providing Insights gathering time
- [#608](https://github.com/openshift/insights-operator/pull/608) Remove PSP gatherer
- [#609](https://github.com/openshift/insights-operator/pull/609) Namespaces with overlapping UIDs - do not store UID ranges
- [#602](https://github.com/openshift/insights-operator/pull/602) Gather documentation update
- [#597](https://github.com/openshift/insights-operator/pull/597) Add list of anonymized data points to documentation
- [#593](https://github.com/openshift/insights-operator/pull/593) Create an alternate IO deployment manifest excluding the NodeSelector
- [#583](https://github.com/openshift/insights-operator/pull/583) implemented fetching rules from a remote server for conditional gathering
- [#591](https://github.com/openshift/insights-operator/pull/591) Update changelog and improve the logic for its generation
- [#590](https://github.com/openshift/insights-operator/pull/590) fix some docs
- [#585](https://github.com/openshift/insights-operator/pull/585) HyperShift - Add required annotation to remaining manifests
- [#582](https://github.com/openshift/insights-operator/pull/582) Send gathering time as metadata field with upload request

### Misc
- [#635](https://github.com/openshift/insights-operator/pull/635) Revert "Implement Prometheus Collector pattern (#607)"
- [#624](https://github.com/openshift/insights-operator/pull/624) Updating ose-insights-operator images to be consistent with ART
- [#616](https://github.com/openshift/insights-operator/pull/616) comply to restricted pod security level
- [#592](https://github.com/openshift/insights-operator/pull/592) Revert "Set default messages & reconcile clusteroperator status conditions"
- [#586](https://github.com/openshift/insights-operator/pull/586) Revert "Set default messages & reconcile clusteroperator status conditions (#584)
- [#577](https://github.com/openshift/insights-operator/pull/577) Updating ose-insights-operator images to be consistent with ART

## 4.10

### Data Enhancement
- [#563](https://github.com/openshift/insights-operator/pull/563) conditional log gathers into a single gather and PrometheusOperatorSyncFailed
- [#557](https://github.com/openshift/insights-operator/pull/557) limit number of containers per namespace
- [#558](https://github.com/openshift/insights-operator/pull/558) Collect Info about Openshift scheduler
- [#551](https://github.com/openshift/insights-operator/pull/551) adding gatherer for collecting silenced alerts
- [#545](https://github.com/openshift/insights-operator/pull/545) alertmanager conditional log gathering
- [#528](https://github.com/openshift/insights-operator/pull/528) changes for collecting tsdb status
- [#529](https://github.com/openshift/insights-operator/pull/529) Gather DVO metrics
- [#517](https://github.com/openshift/insights-operator/pull/517) Collecting node logs
- [#509](https://github.com/openshift/insights-operator/pull/509) Conditional gatherer of logs of unhealthy pods
- [#525](https://github.com/openshift/insights-operator/pull/525) Gather all CostManagementMericsConfig definitions.
- [#508](https://github.com/openshift/insights-operator/pull/508) gather webhook configurations
- [#511](https://github.com/openshift/insights-operator/pull/511) Removing one unnecessary case statement from workload_info
- [#505](https://github.com/openshift/insights-operator/pull/505) Gather jaegers.jaegertracing.io CRs
- [#504](https://github.com/openshift/insights-operator/pull/504) Reduce stacktrace size in logs
- [#492](https://github.com/openshift/insights-operator/pull/492) ApiRequestCount conditional gathering

### Bugfix
- [#534](https://github.com/openshift/insights-operator/pull/534) make projectid and region anonymization consistent
- [#544](https://github.com/openshift/insights-operator/pull/544) fixed a bug with missing metadata
- [#519](https://github.com/openshift/insights-operator/pull/519) unified conditional gatherer api with targeted update edge blocking api
- [#538](https://github.com/openshift/insights-operator/pull/538) Shorter delay in case of HTTP 403 during upload
- [#537](https://github.com/openshift/insights-operator/pull/537) Fix cost management metric resource name
- [#516](https://github.com/openshift/insights-operator/pull/516) Gather all the container logs from related namespaces of degraded clu…
- [#515](https://github.com/openshift/insights-operator/pull/515) obfuscation ovn clusters bug
- [#514](https://github.com/openshift/insights-operator/pull/514) Increment the "insightsclient_request_recvreport_total" metric only w…
- [#507](https://github.com/openshift/insights-operator/pull/507) Anonymize the ImageRegistry storage information also in
- [#495](https://github.com/openshift/insights-operator/pull/495)  Respect user defined proxy's CA cert
- [#497](https://github.com/openshift/insights-operator/pull/497) insightsclient - close response body
- [#494](https://github.com/openshift/insights-operator/pull/494) Fix the error logic in the OCM controller & degrade only…

### Others
- [#575](https://github.com/openshift/insights-operator/pull/575) Minor gathering docs update
- [#574](https://github.com/openshift/insights-operator/pull/574) Remove "InsightsOperatorPullingSCA" TP feature check
- [#565](https://github.com/openshift/insights-operator/pull/565) info alert when the SCA is not available
- [#572](https://github.com/openshift/insights-operator/pull/572) Bump k8s & OpenShift versions
- [#567](https://github.com/openshift/insights-operator/pull/567) Remove unnecessary division into important and failable gatherers
- [#566](https://github.com/openshift/insights-operator/pull/566) Update versions for backports in our gathered data docs
- [#564](https://github.com/openshift/insights-operator/pull/564) recucing configobserver.go cognitive complexity
- [#556](https://github.com/openshift/insights-operator/pull/556) alert about disconnected cluster
- [#562](https://github.com/openshift/insights-operator/pull/562) new cluster operator condition providing info about unavailable SCA certs
- [#524](https://github.com/openshift/insights-operator/pull/524) Cluster version condition
- [#550](https://github.com/openshift/insights-operator/pull/550) workloads info - increase the pods limit a bit
- [#547](https://github.com/openshift/insights-operator/pull/547) Update documentation for PSP gatherer
- [#542](https://github.com/openshift/insights-operator/pull/542) Update docs/arch.md documentation to mention the new gatherers
- [#531](https://github.com/openshift/insights-operator/pull/531) Enhance gathered-data.md
- [#532](https://github.com/openshift/insights-operator/pull/532) Replacing deprecated ioutil
- [#520](https://github.com/openshift/insights-operator/pull/520) Anonymize identity provider attributes in the
- [#498](https://github.com/openshift/insights-operator/pull/498) Refactoring Status controller
- [#513](https://github.com/openshift/insights-operator/pull/513) Reverts "Respect user defined proxy's CA cert"
- [#510](https://github.com/openshift/insights-operator/pull/510) Regenerate changelog & update some gatherers OCP versions
- [#501](https://github.com/openshift/insights-operator/pull/501) Update changelog
- [#499](https://github.com/openshift/insights-operator/pull/499) Fix the sample archive path for the last conditional gatherer
- [#481](https://github.com/openshift/insights-operator/pull/481) Add a script for updating files in the sample archive

### Misc
- [#540](https://github.com/openshift/insights-operator/pull/540) Updating ose-insights-operator images to be consistent with ART
- [#500](https://github.com/openshift/insights-operator/pull/500) OCM controller - change type of the secret
- [#502](https://github.com/openshift/insights-operator/pull/502) Updating ose-insights-operator images to be consistent with ART
- [#491](https://github.com/openshift/insights-operator/pull/491) Updating ose-insights-operator images to be consistent with ART

## 4.9

### Data Enhancement
- [#489](https://github.com/openshift/insights-operator/pull/489) Gather installed PSP names
- [#487](https://github.com/openshift/insights-operator/pull/487) Conditional data gathering validation & refactoring
- [#476](https://github.com/openshift/insights-operator/pull/476) Gather Openshift Logging Stack Data
- [#450](https://github.com/openshift/insights-operator/pull/450) Make obfuscation work with a provided archive
- [#456](https://github.com/openshift/insights-operator/pull/456) Better pod log gathering with offset for stacktrace messages
- [#468](https://github.com/openshift/insights-operator/pull/468) Update the gather functions to collect data from the system namespaces only
- [#433](https://github.com/openshift/insights-operator/pull/433) Conditional gathering
- [#447](https://github.com/openshift/insights-operator/pull/447) fix logs format in sample archive
- [#449](https://github.com/openshift/insights-operator/pull/449) Gather all MachineConfig definitions
- [#446](https://github.com/openshift/insights-operator/pull/446) add egress ips support to anonymizer

### Bugfix
- [#485](https://github.com/openshift/insights-operator/pull/485) Don't try to record an empty Record if gatherClusterConfigV1 fails
- [#473](https://github.com/openshift/insights-operator/pull/473) Insightsreport set corresponding clusteroperator condition correctly
- [#478](https://github.com/openshift/insights-operator/pull/478) Set the disabled state only when the token is removed from the
- [#479](https://github.com/openshift/insights-operator/pull/479) remove the redundant role & rolebinding definition
- [#477](https://github.com/openshift/insights-operator/pull/477) Do not use klog.Fatal
- [#472](https://github.com/openshift/insights-operator/pull/472) Set also the summary operation when updating status
- [#466](https://github.com/openshift/insights-operator/pull/466) fix obfuscation translation table secret manifest
- [#461](https://github.com/openshift/insights-operator/pull/461) fix obfuscation translation table secret
- [#444](https://github.com/openshift/insights-operator/pull/444) MemoryRecord name can be obfuscated & fix case of duplicate records

### Others
- [#488](https://github.com/openshift/insights-operator/pull/488) Update K8s & OpenShift API versions
- [#486](https://github.com/openshift/insights-operator/pull/486) Degraded status in the OCM controller
- [#375](https://github.com/openshift/insights-operator/pull/375) OCM controller - periodically pull the data and update corresponding
- [#460](https://github.com/openshift/insights-operator/pull/460) Remove managedFields from gathered resources
- [#474](https://github.com/openshift/insights-operator/pull/474) Bye bye Pavel
- [#469](https://github.com/openshift/insights-operator/pull/469) Remove ParseJSONQuery function and replace it with unstructured
- [#471](https://github.com/openshift/insights-operator/pull/471) cover tasks_processing.go better
- [#465](https://github.com/openshift/insights-operator/pull/465) Fix installplans sample archive filename
- [#464](https://github.com/openshift/insights-operator/pull/464) Add delete annotation to stale resources
- [#458](https://github.com/openshift/insights-operator/pull/458) Gathered data doc update - add some known previous locations
- [#455](https://github.com/openshift/insights-operator/pull/455) Updating the owners list
- [#463](https://github.com/openshift/insights-operator/pull/463) Enables godox on precommit
- [#454](https://github.com/openshift/insights-operator/pull/454) Update changelog
- [#452](https://github.com/openshift/insights-operator/pull/452) Update versions in the metrics gather documentation

### Misc
- [#457](https://github.com/openshift/insights-operator/pull/457) Updating ose-insights-operator images to be consistent with ART
- [#451](https://github.com/openshift/insights-operator/pull/451) Updating .ci-operator.yaml `build_root_image` from openshift/release

## 4.8

### Data Enhancement
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

### Data Enhancement
- [#327](https://github.com/openshift/insights-operator/pull/327) collect invalid resource name error from logs 
- [#316](https://github.com/openshift/insights-operator/pull/316) Gather list of OLM operator names and versions & minor clean up
- [#319](https://github.com/openshift/insights-operator/pull/319) Gather PersistentVolume definition (if any) used in Image registry st…
- [#291](https://github.com/openshift/insights-operator/pull/291) Gather SAP configuration (SCC & ClusterRoleBinding)
- [#314](https://github.com/openshift/insights-operator/pull/314) collect logs from openshift-sdn-controller pod
- [#309](https://github.com/openshift/insights-operator/pull/309) Collect logs from openshift-sdn namespace
- [#273](https://github.com/openshift/insights-operator/pull/273) Implemented gathering specific logs from openshift apiserver operator
- [#297](https://github.com/openshift/insights-operator/pull/297) Gather netnamespaces network info

### Bugfix
- [#329](https://github.com/openshift/insights-operator/pull/329) Remove StatefulSet gatherer & replace it with gathering "cluster-mon…
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

