---
id: human-grafana-provisioner-readme
label: likely_human
bucket: human
source: github.com/mad01/grafana-provisioner README.md, commit fd6a0b51adce (2018-06-26)
license: owned by the repo author
generator: ""
words: 148
notes: Overview and problem statement of a 2018 CLI README; short and list-led, the kind of README intro AI is asked to write.
---
# grafana-provisioner
The project is currently pre-alpha and it is expected that breaking changes to the API will be made in the upcoming releases.

### Overview 
Small comandline tool to manage N grafana deployments backed by a mysql database as a dashboard store for grafana. When running the tool will. 
* create a database for every team to be used by grafana
* provision a namespace, serviceaccount, ingress, service, deployemnt in k8s

if there is changes in the manifests the changes will be applied in the cluster to all teams in the config, or the single team passed by flag. 


### Problem statement
having N deployments of grafana managed will result is image version drift, having a solution for git sync like the current solution is for teams to manage deployments is not a good user experience. and maintaining using helm or equivalent will need to much tooling.
