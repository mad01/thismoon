---
id: human-totem-readme
label: likely_human
bucket: human
source: github.com/mad01/totem README.md, commit 8414a54d802c (2018-09-20)
license: owned by the repo author
generator: ""
words: 184
notes: Problem statement and solution notes from a 2018 Go service README; fragmentary, typo-laden engineering notes with a numbered list.
---
### Problem statement
1. Managment of kube configs when multiple orgs and teams are involved.
2. Not having access to configure and select a auth provider for the cluster. 
3. Having short lived kube configs
4. Having individual kube configs 
5. Having having the option to use different cluster roles for different individuals


### Solution
To allow the solution to run both when we have access to the master and can configure a auth provider and when not. Using the service accounts as a base for the indivdual kube configs and using the service account token and cert to generate a kube config. When the kube config have passed the allowed ttl the service account is removed and access is removed.

Creating a config. when you request a new config you get one generated with the configures lifetime. you can have multiple configs active at one time
Deleting a config. when you delete a config it will remove all configs created for your user. 
Adding a new user. you need to update the deployment config map with the new user, and recreate the pod
