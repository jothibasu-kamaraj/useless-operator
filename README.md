# useless-operator

`useless-operator` is a tool which helps to detect orphaned resources in a Kubernetes cluster.  

The main idea is to detect unused pods and calculate how much resources they consumes (CPU/memory [requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/)).

Statistics are based on data collected using [Prometheus](https://github.com/prometheus/prometheus) and take into 
account the selected observation period.

### Example output

```bash
I0130 11:34:56.895982   27862 kubernetes.go:53] There are 34 nodes in the cluster
I0130 11:36:37.144725   27862 useless-operator.go:116] Requested period: 180 hours, Observed period: 180 hours, Unused PODs count (no traffic): 87 in 69 namespaces
I0130 11:36:37.144759   27862 useless-operator.go:119] Reqests: CPU: 17, memory (MB): 18368
``` 

### Features/Roadmap:
- Detect orphaned Pods without outgoing traffic (/)
- Detect orphaned Ingresses and their Pods (/)
- Detect pods which are in permanent failed state (they are consumes resources too)
- Calculate resources (CPU and memory "Requests") of the orphaned resources (/)
- Detect "parents" of orphaned resources (Deployments, DaemonSets etc.)
- Expose metrics into Prometheus
- "Operator" mode
- Helm chart
- Grafana dashboard