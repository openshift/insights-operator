# Note: This CHANGELOG is only for the changes in insights operator. Please see OpenShift release notes for official changes

## 4.7

- [#279](https://github.com/openshift/insights-operator/pull/279) Refactoring record and gatherer
- [#297](https://github.com/openshift/insights-operator/pull/297) Gather netnamespaces network info
- [#292](https://github.com/openshift/insights-operator/pull/292) Update initial waiting times and give TestIsIOHealthy more time
- [#289](https://github.com/openshift/insights-operator/pull/289) Check context status when checking container is running OK
- [#275](https://github.com/openshift/insights-operator/pull/275) Adding a metrics report to IO gatherers
- [#270](https://github.com/openshift/insights-operator/pull/270) First check IO container status and optionally delay first gathering
- [#281](https://github.com/openshift/insights-operator/pull/281) Fix bug in statefulset gatherer & minor doc fix
- [#277](https://github.com/openshift/insights-operator/pull/277) Annotate manifests for single-node-developer cluster profile
- [#266](https://github.com/openshift/insights-operator/pull/266) Collect complete spec info for cluster operator resources
- [#274](https://github.com/openshift/insights-operator/pull/274) Add hostsubnet to sample archive & fix bug in the hostsubnet gathering
- [#264](https://github.com/openshift/insights-operator/pull/264) Reuse archives & refactor archive checks + some fixes
- [#272](https://github.com/openshift/insights-operator/pull/272) Fix clusteroperators serialization
- [#271](https://github.com/openshift/insights-operator/pull/271) Init health status metrics to distinguish no report state vs 0 problems
- [#174](https://github.com/openshift/insights-operator/pull/174) Improve container image collection
- [#230](https://github.com/openshift/insights-operator/pull/230) Add IO Architecture doc and metrics sample
- [#257](https://github.com/openshift/insights-operator/pull/257) Separating the gather logic into separate files
- [#259](https://github.com/openshift/insights-operator/pull/259) Add IBM Cloud managed annotations to CVO manifests
- [#246](https://github.com/openshift/insights-operator/pull/246) IO archive contains more records of than is the limit
- [#223](https://github.com/openshift/insights-operator/pull/223) Gather clusteroperator resources
- [#235](https://github.com/openshift/insights-operator/pull/235) add current profile annotations to CVO manifests
- [#241](https://github.com/openshift/insights-operator/pull/241) Added TestArchiveUploadedAndResultReceived
- [#234](https://github.com/openshift/insights-operator/pull/234) Simplify/generalize host subnet pattern

## 4.6

- [#261](https://github.com/openshift/insights-operator/pull/261) Fixes records index on diskrecorder
- [#221](https://github.com/openshift/insights-operator/pull/221) Add the namespace to the gatherers reports to avoid conflicts
- [#276](https://github.com/openshift/insights-operator/pull/276) Add hostsubnet to sample archive & fix bug in the hostsubnet gathering
- [#226](https://github.com/openshift/insights-operator/pull/226) Fixes policyClient and the corresponding config
- [#197](https://github.com/openshift/insights-operator/pull/197) Adds info about sample archive in README.md
- [#185](https://github.com/openshift/insights-operator/pull/185) Adds gatherer for PodDistributionBudget
- [#184](https://github.com/openshift/insights-operator/pull/184) Limit the maximum number of CSR
- [#175](https://github.com/openshift/insights-operator/pull/175) Adds cluster version into the User-Agent header
- [#165](https://github.com/openshift/insights-operator/pull/165) Added log checker, which is more flexible than old function
- [#182](https://github.com/openshift/insights-operator/pull/182) Automate TestArchiveContains::HostsSubnet & 2 more
- [#193](https://github.com/openshift/insights-operator/pull/193) Make gen-doc work outside of GOPATH
- [#183](https://github.com/openshift/insights-operator/pull/183) Gather MachineSet info
- [#188](https://github.com/openshift/insights-operator/pull/188) Do not return CRD not found error, just log it
