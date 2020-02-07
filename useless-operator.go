package main

import (
	"bufio"
	"flag"
	"fmt"
	"k8s.io/klog"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"strconv"

	prom "github.com/Nastradamus/useless-operator/pkg/prometheus"
	ukube "github.com/Nastradamus/useless-operator/pkg/ukubernetes"
)

func main() {
	// Parse and validate flags, setup logging
	var (
		v                 = flag.Int("v", 1, "Verbosity level (klog).")
		profile           = flag.Bool("profile", false, "Enable profiling on http://0.0.0.0:6060")
		period            = flag.Int("period", 6, "Observation period in hours.")
		promAddr          = flag.String("prom-uri", "", "Prometheus URI (e.g. http://localhost:9091).")
		runOutsideCluster = flag.Bool("run-outside-cluster", false, "Set this flag when running "+
			"outside of the cluster.")
	)
	var Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)

	flag.Parse()

	klog.InitFlags(klogFlags)
	klog.SetOutput(os.Stdout)

	verbosity := klogFlags.Lookup("v")
	verbosity.Value.Set(strconv.Itoa(*v))

	klog.V(0).Infof("Verbosity level set to %v", klogFlags.Lookup("v").Value)

	if *profile {
		klog.V(0).Infof("Profiling enabled. URL: http://0.0.0.0:6060/debug/pprof/" +
			"Usage:\n" +
			"http://localhost:6060/debug/pprof/\n" +
			"go tool pprof http://0.0.0.0:6060/debug/pprof/heap\n" +
			"go tool pprof http://0.0.0.0:6060/debug/pprof/profile?seconds=30\n" +
			"go tool pprof http://0.0.0.0:6060/debug/pprof/block\n" +
			"wget http://0.0.0.0:6060/debug/pprof/trace?seconds=5\n" +
			"go tool pprof http://0.0.0.0:6060/debug/pprof/mutex\n" +
			"To view all available profiles, open http://0.0.0.0:6060/debug/pprof/ in your browser. ")
		go func() {
			klog.Infoln(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}

	// Check Prometheus endpoint's syntax
	_, err := url.ParseRequestURI(*promAddr)
	if err != nil {
		Usage()
		klog.Exit(err)
	}

	// Get kubernetes config
	config, err := ukube.GetConfig(*runOutsideCluster)
	if err != nil {
		klog.Exit(err)
	}

	klog.V(0).Infof("Starting useless-operator...")

	// Get tested k8s client
	kClient, err := ukube.GetKClient(config)
	if err != nil {
		klog.Exit(err)
	}

	//ololo, err := kClient.AppsV1().Deployments("ops").List(metav1.ListOptions{})
	//if err != nil {
	//	log.Printf("ERROR: %v", err)
	//}

	// podCpu, podMem, err := ukube.GetPodRequests("ops-test", "busybox1", kClient)
	// fmt.Printf("\nCPU: %v, memory: %v\n\n", podCpu, podMem)

	// Query Prometheus
	promQueryPods := `sum(rate(container_network_transmit_packets_total{container_name="POD", 
				service="prometheus-operator-kubelet"}[1h])) by (namespace, pod_name) == 0`
	promPodsMap, observedPeriod, err := prom.GetUnusedResources(*promAddr, *period, promQueryPods)
	if err != nil {
		klog.Warningf("%v", err)
	}

	// Estimate resources of unused pods during given observation period
	UselessPodsCnt := 0
	ObservedNamespacesCnt := 0
	var allPodsCpu int64 // milli
	var allPodsMem int64 // bytes
	for namespace := range promPodsMap {
		// Pods
		for pod := range promPodsMap[namespace] {
			UselessPodsCnt++
			podCpu, podMem, err := ukube.GetPodRequests(namespace, pod, kClient)
			if err != nil {
				// pod may disappear
				klog.V(3).Infof("%v", err)
				continue
			}

			allPodsCpu += podCpu
			allPodsMem += podMem

			klog.V(3).Infof("Namespace: %v, POD: %v, Reqests: mCPU: %v, memory (bytes): %v\n", namespace,
				pod, podCpu, podMem)
		}
		ObservedNamespacesCnt++
	}

	klog.V(1).Infof("Requested period: %v hours, Observed period: %v hours, "+
		"Unused PODs count (no traffic): %v in %v promPodsMap\n", *period, observedPeriod, UselessPodsCnt,
		len(promPodsMap))
	klog.V(1).Infof("Reqests: CPU: %v, memory (MB): %v\n", allPodsCpu/1000, allPodsMem/1024/1024)

	// Get unused ingresses

	promQueryIngresses := "sum(rate(nginx_ingress_controller_requests[1h])) by (ingress, exported_namespace) == 0"
	promIngressesMap, observedPeriod, err := prom.GetUnusedResources(*promAddr, *period, promQueryIngresses)
	if err != nil {
		klog.Warningf("%v", err)
	}

	klog.V(8).Infof("Ingresses map: \n %v", promIngressesMap)

	// Don't exit if we want profiling (for now)
	if *profile {
		fmt.Print("Program stopped. Type something to exit: ")
		input := bufio.NewScanner(os.Stdin)
		input.Scan()
		fmt.Println(input.Text())
	}
}

// TODO:
// - Cache
// - Flush logs on exit ?
// - Get pod's labels to compare
// - find selectors over Deployments, Daemonsets, StatefulSets, jobs, etc. (compare maps)
