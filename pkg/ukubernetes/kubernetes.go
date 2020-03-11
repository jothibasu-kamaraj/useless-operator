package ukubernetes

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
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
	for i := range []int{1, 2, 3} {
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

// GetSvcSelectorByIngressBackend returns service's selector
func GetSvcSelectorByIngressBackend(kClient *kubernetes.Clientset, namespace string, ServiceName string) (map[string]string, error) {
	svc, err := kClient.CoreV1().Services(namespace).Get(ServiceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return svc.Spec.Selector, nil
}

// GetPodsBySelector
func GetPodsBySelector(kClient *kubernetes.Clientset, namespace string, selector map[string]string) (*v1.PodList, error) {

	// Obtain string form of selector
	lp := labels.Set(selector).String()


	pods, err := kClient.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: lp})
	if err != nil {
		return nil, err
	}

	return pods, nil
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
func GetPodRequests(kClient *kubernetes.Clientset, namespace, podName string) (cpu int64, mem int64, err error) {

	pod, err := kClient.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	var podCpu int64
	var podMem int64
	for _, containerName := range pod.Spec.Containers {
		qContainerCpu := containerName.Resources.Requests["cpu"]
		containerCpu := qContainerCpu.MilliValue()

		podCpu += containerCpu

		qContainerMem := containerName.Resources.Requests["memory"]
		containerMem := qContainerMem.Value()

		podMem += containerMem
	}

	return podCpu, podMem, nil
}

// GetPodDeployment return's pod's Deployment object
func GetPodDeployment(kClient *kubernetes.Clientset, namespace, podName string) (deployments []string, err error) {
	pod, err := kClient.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var ds string

	if len(pod.OwnerReferences) > 0 {
		for _, ref := range pod.OwnerReferences {
			if ref.Kind != "ReplicaSet" {
				continue
			} else {
				replicaSet, err := kClient.AppsV1().ReplicaSets(namespace).Get(ref.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				if replicaSet.OwnerReferences[0].Kind == "Deployment" {
					ds = replicaSet.OwnerReferences[0].Name
				}
			}
			if ds == "" {
				continue
			}
			deployments = append(deployments, ds)
		}
	} else {
		return nil, err
	}

	return deployments, nil
}