# useless-operator

`useless-operator` is a tool which helps to detect orphaned resources (such as Pods, Deployments, etc.) in a Kubernetes cluster.  

The main idea is to detect unused pods and calculate how much resources they consumes (CPU/memory [requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/)).

Statistics are based on data collected by [Prometheus](https://github.com/prometheus/prometheus) and taken into 
account the selected observation period.

### Example output

```bash
./useless-operator --prom-uri http://devpromstore.example.com --period 168 --run-outside-cluster -v 3

I0311 17:52:30.045145   81149 useless-operator.go:43] Verbosity level set to 3
I0311 17:52:30.045595   81149 kubernetes.go:51] Kubernetes config Location: /Users/nas/.kube/config
I0311 17:52:30.048293   81149 useless-operator.go:73] Starting useless-operator...
I0311 17:52:30.160118   81149 kubernetes.go:87] There are 34 nodes in the cluster
I0311 17:52:30.160145   81149 useless-operator.go:100] Querying Prometheus for unused pods...
I0311 17:53:19.172218   81149 useless-operator.go:110] Estimating resources of unused pods during given observation period (querying API)...
I0311 17:53:52.016034   81149 useless-operator.go:154] Requested period: 168 hours, Observed period: 168 hours, Unused PODs count (no traffic): 95 of 81 namespaces
I0311 17:53:52.016075   81149 useless-operator.go:157] Reqests of unused pods: CPU: 16.725, memory (MB): 17796
I0311 17:53:52.016089   81149 useless-operator.go:164] Getting unused ingresses...
I0311 17:55:05.907484   81149 useless-operator.go:175] 'Unused Ingresses' observed period: 168
I0311 17:55:05.907510   81149 useless-operator.go:179] Getting backends of unused ingresses...
I0311 17:55:05.907526   81149 useless-operator.go:226]

...skipped...

I0311 17:55:05.907615   81149 useless-operator.go:238] Use the following commands to free resources in the cluster:

kubectl -n kube-public scale deployment clickhouse-client --replicas=0
kubectl -n platform scale deployment mok-storage-scheduler-staging --replicas=0
kubectl -n platform scale deployment webrtc-widget-node1-stage --replicas=0
kubectl -n platform scale deployment webrtc-echotest-node1-stage --replicas=0
kubectl -n platform scale deployment com-ins-front-dev --replicas=0
kubectl -n platform scale deployment sbs-mock-deployment --replicas=0


``` 

### Features/Roadmap:
- [x] Detect orphaned Pods without outgoing traffic (/)
- [x] Detect orphaned Ingresses and their Pods (/)
- [ ] Detect pods which are in permanent failed state (they are consumes resources too)
- [x] Calculate resources (CPU and memory "Requests") of the orphaned resources (/)
- Detect "parents" of orphaned resources:
  - [x] Deployments (/)
- [ ] Expose metrics into Prometheus
- [ ] "Operator" mode
- [ ] Helm chart
- [ ] Grafana dashboard
