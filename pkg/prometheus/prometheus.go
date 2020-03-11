package prometheus

import (
	"context"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type Namespace string
type Element string  // Pod/Ingress/etc.

// GetUnusedIngresses structures and types
type Ingress string
type IngNamespace string
type Host string
type Path string

// IngressBackend
type IngressBackend struct {
	// Specifies the name of the referenced service.
	ServiceName string `json:"serviceName" protobuf:"bytes,1,opt,name=serviceName"`

	// Specifies the port of the referenced service.
	ServicePort intstr.IntOrString `json:"servicePort" protobuf:"bytes,2,opt,name=servicePort"`
}

type IngressMap struct {
	M map[IngNamespace]map[Ingress]map[Host]map[Path]IngressBackend
}

// END OF GetUnusedIngresses structures

// AddIntoIngMap is a method for IngMapInterface
// Fills maps from Prometheus. Example of metric:
// {exported_namespace="polo",host="polo-stage.test.com",ingress="polo-api-staging-p8080-1496620443",path="/"}
func (resultMap *IngressMap) AddIntoIngMap(ns IngNamespace, ing Ingress, host Host, path Path) {
	// implements insert into `map[IngNamespace]map[Ingress]map[Host]map[Path]IngressBackend`

	if resultMap.M == nil {
		resultMap.M = make(map[IngNamespace]map[Ingress]map[Host]map[Path]IngressBackend)
	}

	// Try to read map[IngNamespace]
	nsMap, ok := resultMap.M[ns]
	if !ok {
		// Try to read outer map
		nsMap = make(map[Ingress]map[Host]map[Path]IngressBackend)
		resultMap.M[ns] = nsMap
		// Try to read deeper map `map[Ingress]map[Host]map[Path]IngressBackend`
		iMap, ok := resultMap.M[ns][ing]
		if !ok {
			iMap = make(map[Host]map[Path]IngressBackend)
			resultMap.M[ns][ing] = iMap
			// Try to read deeper map `map[Host]map[Path]IngressBackend`
			hMap, ok := resultMap.M[ns][ing][host]
			if !ok {
				hMap = make(map[Path]IngressBackend)
				resultMap.M[ns][ing][host] = hMap
				// Fill backend structure with empty values
				empt := intstr.IntOrString{}
				resultMap.M[ns][ing][host][path] = IngressBackend{"", empt}
			}
		}
	}
}

// GetLabelVal returns given label's value from Prometheus string. Exmaple of string:
// {exported_namespace="polo",host="polo-stage.test.com",ingress="polo-api-staging-p8080-1496620443",path="/"}
func GetLabelVal(str *string, label string) string {
	if label == "" {
		return ""
	}

	labelStartPos := strings.Index(*str, label)
	valueStartPos := labelStartPos + len(label) + 2

	valueEndPos := valueStartPos + strings.Index((*str)[valueStartPos:], `"`)

	return (*str)[valueStartPos:valueEndPos]
}

// MapAdd adds element into map of map
func MapAdd(m map[Namespace]map[Element]string, ns Namespace, elem Element, deployment string) {
	// Try to read outer map by key (ns)
	mm, ok := m[ns]
	if !ok {
		// Make empty inner map
		mm = make(map[Element]string)
		// Create outer map key as an empty inner map
		m[ns] = mm
	}
	// Add element into inner map
	mm[elem] = deployment
}

// GetUnusedResources returns map of unused resources with real observed period in hours.
// This function works only for metrics with two elements. Example:
// `sum(rate(nginx_ingress_controller_requests[1h])) by (ingress, exported_namespace) == 0`
func GetUnusedResources(promAddr string, maxSteps int, promQuery string) (map[Namespace]map[Element]string, int, error) {

	// Resulting map to return
	var resultMap = map[Namespace]map[Element]string{}

	// Setup Prometheus client
	client, err := api.NewClient(api.Config{
		Address: promAddr,
	})
	if err != nil {
		return map[Namespace]map[Element]string{}, 0, err
	}
	v1api := v1.NewAPI(client)

	observedPeriod := 0
	// Query Prometheus with 1 hour shift backwards
	for step := 0; step < maxSteps; step++ {
		startTime := time.Now().Add(-1 * time.Duration(step) * time.Hour)

		// Setup connection to Prometheus
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		// Query Prometheus (opens connection)
		result, warnings, err := v1api.Query(ctx, promQuery, startTime)
		if err != nil {
			return map[Namespace]map[Element]string{}, 0, err
		}
		if len(warnings) > 0 {
			klog.Warningf("Warnings: %v\n", warnings)
		}
		// Close current connection due to free memory on the Prometheus instance after each query
		cancel()

		// Split results to strings
		strs := strings.Split(result.String(), "\n")

		observedPeriod = step + 1 // step by 1 hour
		// Don't read empty strings (1 is empty array of strings)
		if len(strs) < 2 {
			break
		}

		// Temporary map for current step
		var tempMap = map[Namespace]map[Element]string{}

		// Parse strings and add to map
		for _, str := range strs {
			// Cut part without values
			cut := strings.Split(str, " => ")

			// Don't panic if no result
			if cut[0] == "{}" {
				continue
			}

			// Take queried resource and namespace names, avoiding regexps

			// TODO: make it independent of keys order returned by Prometheus
			resStartPos := strings.LastIndex(cut[0], `="`)
			resEndPos := strings.LastIndex(cut[0], `"}`)

			namespaceStartPos := strings.Index(cut[0], `="`)
			namespaceEndPos := strings.LastIndex(cut[0], `",`)

			namespace := Namespace(cut[0][namespaceStartPos+2 : namespaceEndPos])
			res := Element(cut[0][resStartPos+2 : resEndPos])

			MapAdd(tempMap, namespace, res, "")
		}

		if step == 0 {
			resultMap = tempMap
		}

		// Delete from resultMap values which are not exists in tempMap
		// TODO: understand why it's slow here in debug log mode (buffered i/o while logging?)
		for ns, resMap := range resultMap {
			if step == 0 {
				break
			}
			for res := range resMap {
				// Try to find element in current tempMap
				_, ok := tempMap[ns][res]
				if !ok {
					// If we see non-empty result on any step, consider this resource as "useful"
					delete(resultMap[ns], res)
					klog.V(8).Infof("Dleteted from resultMap: ns: %v, res %v\n", ns, res)
				}
			}
		}
		// TODO: delete empty namespaces. This is a bug!
	}

	return resultMap, observedPeriod, nil
}

// GetUnusedIngresses
// `sum(rate(nginx_ingress_controller_request_size_count[1h])) by (exported_namespace, ingress, host, path) == 0`
func (resultMap *IngressMap) GetUnusedIngresses(promAddr string, maxSteps int, promQuery string) (observedPeriod int, err error) {

	// Setup Prometheus client
	client, err := api.NewClient(api.Config{
		Address: promAddr,
	})
	if err != nil {
		return 0, err
	}
	v1api := v1.NewAPI(client)

	observedPeriod = 0
	// Query Prometheus with 1 hour shift backwards
	for step := 0; step < maxSteps; step++ {
		startTime := time.Now().Add(-1 * time.Duration(step) * time.Hour)

		// Setup connection to Prometheus
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		// Query Prometheus (opens connection)
		result, warnings, err := v1api.Query(ctx, promQuery, startTime)
		if err != nil {
			return 0, err
		}
		if len(warnings) > 0 {
			klog.Warningf("Warnings: %v\n", warnings)
		}
		// Close current connection due to free memory on the Prometheus instance after each query
		cancel()

		// Split results to strings
		strs := strings.Split(result.String(), "\n")

		observedPeriod = step + 1 // step by 1 hour
		// Don't read empty strings (1 is empty array of strings)
		if len(strs) < 2 {
			break
		}

		// Temporary map for current step
		var tempMap IngressMap

		// Parse strings and add to map
		for _, str := range strs {
			// Cut part without values
			cut := strings.Split(str, " => ")

			// Don't panic if no result
			if cut[0] == "{}" {
				continue
			}

			// Extract values from query result
			// {exported_namespace="polo",host="polo-stage.test.com",ingress="polo-api-staging-p8080-1496620443",path="/"}

			namespace := GetLabelVal(&str, "exported_namespace")
			ingress := GetLabelVal(&str, "ingress")
			host := GetLabelVal(&str, "host")
			path := GetLabelVal(&str, "path")
			// TODO: validate len of each value and skip failed strings (now work-arounded in the Prometheus query)

			tempMap.AddIntoIngMap(IngNamespace(namespace), Ingress(ingress), Host(host), Path(path))
		}

		// Fill empty result map
		if step == 0 || resultMap.M == nil {
			*resultMap = tempMap
		}

		// Delete from resultMap values which are not exists in tempMap
		for ns, ingMap := range resultMap.M {
			if step == 0 {
				break
			}

			// Delete namespace from resultMap if not found in any iteration of cycle
			_, ok := tempMap.M[ns]
			if !ok {
				resultMap.M[ns] = nil
				continue
			}

			for ing, hostMap := range ingMap {
				// Delete ingress from resultMap if not found in any iteration of cycle
				_, ok := tempMap.M[ns][ing]
				if !ok {
					delete(resultMap.M[ns], ing)
					continue
				}
				for host, pathMap := range hostMap {
					// Delete host from resultMap if not found in any iteration of cycle
					_, ok := tempMap.M[ns][ing]
					if !ok {
						delete(resultMap.M[ns][ing], host)
						continue
					}
					for path := range pathMap {
						// Delete path from resultMap if not found in any iteration of cycle
						_, ok := tempMap.M[ns][ing][host][path]
						if !ok {
							delete(resultMap.M[ns][ing][host], path)
							continue
						}
					}
				}
			}
		}
	}

	return observedPeriod, nil
}