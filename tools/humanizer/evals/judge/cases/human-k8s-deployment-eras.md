---
id: human-k8s-deployment-eras
label: likely_human
bucket: human
source: https://github.com/kubernetes/website/blob/20189c20002fbcb0c207cad4b26066df729789be/content/en/docs/concepts/overview/what-is-kubernetes.md, commit 20189c20002f (2019-06-20)
license: CC BY 4.0 (kubernetes/website), quoted with attribution
generator: ""
words: 260
notes: Committee-edited docs prose with bold era lead-ins and stock phrases (and much more), the corporate-docs register a judge can mistake for AI.
---
**Traditional deployment era:**
Early on, organizations ran applications on physical servers. There was no way to define resource boundaries for applications in a physical server, and this caused resource allocation issues. For example, if multiple applications run on a physical server, there can be instances where one application would take up most of the resources, and as a result, the other applications would underperform. A solution for this would be to run each application on a different physical server. But this did not scale as resources were underutilized, and it was expensive for organizations to maintain many physical servers.

**Virtualized deployment era:**  As a solution, virtualization was introduced. It allows you to run multiple Virtual Machines (VMs) on a single physical server's CPU. Virtualization allows applications to be isolated between VMs and provides a level of security as the information of one application cannot be freely accessed by another application.

Virtualization allows better utilization of resources in a physical server and allows better scalability because an application can be added or updated easily, reduces hardware costs, and much more.

Each VM is a full machine running all the components, including its own operating system, on top of the virtualized hardware.

**Container deployment era:** Containers are similar to VMs, but they have relaxed isolation properties to share the Operating System (OS) among the applications. Therefore, containers are considered lightweight. Similar to a VM, a container has its own filesystem, CPU, memory, process space, and more. As they are decoupled from the underlying infrastructure, they are portable across clouds and OS distributions.
