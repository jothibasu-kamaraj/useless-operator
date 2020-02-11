package ukubernetes

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Ingress rule
type Rule struct {
	Host  string
	Paths []string
}

// Ingress
type Ingress struct {
	Name      string
	Namespace string
	Rules     []Rule
}

// IngressBackend describes all endpoints for a given service and port.
type IngressBackend struct {
	// Specifies the name of the referenced service.
	ServiceName string `json:"serviceName" protobuf:"bytes,1,opt,name=serviceName"`

	// Specifies the port of the referenced service.
	ServicePort intstr.IntOrString `json:"servicePort" protobuf:"bytes,2,opt,name=servicePort"`
}

// GetConfig returns k8s Config struct
func GetConfig(runOutsideCluster bool) (*rest.Config, error) {

	kubeConfigLocation := ""
	var config *rest.Config
	var err error
	if runOutsideCluster {
		homeDir := os.Getenv("HOME")
		kubeConfigLocation = filepath.Join(homeDir, ".kube", "config")
		klog.V(1).Infof("Kubernetes config Location: %v\n", kubeConfigLocation)
		// Use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigLocation)
		if err != nil {
			return nil, err
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		klog.V(1).Infof("Running inside Kubernetes cluster")
	}

	return config, nil
}

// GetKClient returns *kubernetes.Clientset with tested connection
func GetKClient(restconfig *rest.Config) (*kubernetes.Clientset, error) {
	// Setup k8s client
	kClient, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return kClient, err
	}

	var gotNodes = false
	for i, _ := range []int{1, 2, 3} {
		// Test connection to k8s API server
		nodes, err := kClient.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			klog.Warningf("(try #%d of %d)\n", i+1, len([]int{1, 2, 3}))
			time.Sleep(5 * time.Second)

			continue
		} else {
			gotNodes = true
			klog.V(3).Infof("There are %d nodes in the cluster\n", len(nodes.Items))

			break
		}
	}
	if !gotNodes {
		klog.Exit("FATAL: Can't access cluster")
	}

	return kClient, err
}

// GetMClient returns *metrics.Clientset with tested connection
//func GetMClient(restconfig *rest.Config) (*metrics.Clientset, error) {
//	// Setup k8s client
//	mClient, err := metrics.NewForConfig(restconfig)
//	if err != nil {
//		log.Panic("FATAL: %v\n", err)
//	}
//
//	return mClient, err
//}

// GetSvcSelector returns ingresse's selector
func GetSvcSelector(kClient kubernetes.Clientset, namespace, ingress string) map[string]string {
	svc, err := kClient.CoreV1().Services(namespace).Get(ingress, metav1.GetOptions{})
	if err != nil {
		log.Printf("ERROR: %v\n", err)
		// Avoid races "...if service was deleted while querying API"
		return nil
	}

	return svc.Spec.Selector
}

// GetIngressBackend returns ingress backend by specific host and path
func GetIngressBackend(kClient *kubernetes.Clientset, namespace, ingress, host, path string) (backend IngressBackend, err error) {

	ingressStruct, err := kClient.ExtensionsV1beta1().Ingresses(namespace).Get(ingress, metav1.GetOptions{})
	if err != nil {
		return backend, err
	}

	found := false
	for _, rule := range ingressStruct.Spec.Rules {
		if rule.Host == host {
			// TODO: fix empty paths
			for _, tPath := range rule.IngressRuleValue.HTTP.Paths {
				if tPath.Path == path || tPath.Path == "" {
					backend = IngressBackend(tPath.Backend)
					found = true
				}
			}
		}
	}

	if !found {
		return backend, fmt.Errorf("not found")
	}

	return backend, nil
}

// GetPodsCpuReq returns CPU and memory requests
// 0.100 CPU mean "1/10 of 1 core CPU time".
// memory units is bytes
func GetPodRequests(namespace, pod string, kClient *kubernetes.Clientset) (int64, int64, error) {
	// TODO: use batching here
	pods, err := kClient.CoreV1().Pods(namespace).Get(pod, metav1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	var podCpu int64
	var podMem int64
	for _, containerName := range pods.Spec.Containers {
		qContainerCpu := containerName.Resources.Requests["cpu"]
		containerCpu := qContainerCpu.MilliValue()

		podCpu += containerCpu

		qContainerMem := containerName.Resources.Requests["memory"]
		containerMem := qContainerMem.Value()

		podMem += containerMem
	}

	return podCpu, podMem, nil
}
