package prometheus

import (
	"context"
	"k8s.io/klog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// mapAdd adds ingresses to map of map
func mapAdd(m map[string]map[string]bool, namespace, ingress string) {
	// Try to read outer map by key (namespace)
	mm, ok := m[namespace]
	if !ok {
		// Make inner map
		mm = make(map[string]bool)
		// Create outer map key as empty inner map
		m[namespace] = mm
	}
	// Add ingress into inner map
	mm[ingress] = true
}

// GetUnusedIngresses returns map of unused ingresses with real observed period in hours
func GetUnusedIngresses(promAddr string, maxSteps int) (map[string]map[string]bool, int, error) {

	// Resulting map to return from GetUnusedIngresses
	var resultMap = map[string]map[string]bool{}

	// Setup Prometheus client
	client, err := api.NewClient(api.Config{
		Address: promAddr,
	})
	if err != nil {
		return map[string]map[string]bool{}, 0, err
	}
	v1api := v1.NewAPI(client)

	// Get 1 hour with default resolution from aggregated by 5 minute requests (no data loss)
	query := "sum(rate(nginx_ingress_controller_requests[1h])) by (ingress, exported_namespace) == 0"

	observedPeriod := 0
	// Query Prometheus with 1 hour shift backwards
	for step := 0; step < maxSteps; step++ {
		startTime := time.Now().Add(-1 * time.Duration(step) * time.Hour)

		// Setup connection to Prometheus
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		//fmt.Printf("Iteration number: %d, Time: %v\n", step, starttime)

		// Query Prometheus (opens connection)
		result, warnings, err := v1api.Query(ctx, query, startTime)
		if err != nil {
			return map[string]map[string]bool{}, 0, err
		}
		if len(warnings) > 0 {
			klog.Warningf("Warnings: %v\n", warnings)
		}
		// Close current connection due to free memory on the Prometheus instance after each query
		cancel()

		// Split results to strings
		strs := strings.Split(result.String(), "\n")

		observedPeriod = step + 1
		// Don't read empty strings (1 is empty array of strings)
		if len(strs) < 2 {
			break
		}

		// Temporary map for current step
		var tempMap = map[string]map[string]bool{}

		// Parse strings and add to map
		for _, str := range strs {
			// Cut part without values
			cut := strings.Split(str, " => ")

			// Don't panic if no result
			if cut[0] == "{}" {
				continue
			}

			// TODO: make it independent of keys order returned by Prometheus
			ingStartPos := strings.LastIndex(cut[0], `="`)
			ingEndPos := strings.LastIndex(cut[0], `"}`)

			namespaceStartPos := strings.Index(cut[0], `="`)
			namespaceEndPos := strings.LastIndex(cut[0], `",`)

			namespace := cut[0][namespaceStartPos+2:namespaceEndPos]
			ingress := cut[0][ingStartPos+2:ingEndPos]

			mapAdd(tempMap, namespace, ingress)
		}

		if step == 0 {
			resultMap = tempMap
		}

		// Delete from resultMap values which are not exists in tempMap
		for ns, ingressMap := range resultMap {
			if step == 0 { break }
			for ingress := range ingressMap {
				// log.Printf("ns: %v, ing: %v\n", ns, ingress)

				// Try to find ingress in
				_, ok := tempMap[ns][ingress]
				if !ok {
					delete(resultMap[ns], ingress)
					//log.Printf("Dleteted: ns: %v, ingress %v\n", ns, ingress)
				}
			}
		}
	}

	return resultMap, observedPeriod, nil
}

// GetUnusedPods returns map of unused pods with real observed period in hours
func GetUnusedPods(promAddr string, maxSteps int) (map[string]map[string]bool, int, error) {

	// Resulting map to return
	var resultMap = map[string]map[string]bool{}

	// Setup Prometheus client
	client, err := api.NewClient(api.Config{
		Address: promAddr,
	})
	if err != nil {
		return map[string]map[string]bool{}, 0, err
	}
	v1api := v1.NewAPI(client)

	// Get pods without outgoing traffic for 1 hour period
	query := `sum(rate(container_network_transmit_packets_total{container_name="POD", 
				service="prometheus-operator-kubelet"}[1h])) by (namespace, pod_name) == 0`

	observedPeriod := 0
	// Query Prometheus with 1 hour shift backwards
	for step := 0; step < maxSteps; step++ {
		startTime := time.Now().Add(-1 * time.Duration(step) * time.Hour)

		// Setup connection to Prometheus
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		// Query Prometheus (opens connection)
		result, warnings, err := v1api.Query(ctx, query, startTime)
		if err != nil {
			return map[string]map[string]bool{}, 0, err
		}
		if len(warnings) > 0 {
			klog.Warningf("Warnings: %v\n", warnings)
		}
		// Close current connection due to free memory on the Prometheus instance after each query
		cancel()

		// Split results to strings
		strs := strings.Split(result.String(), "\n")

		observedPeriod = step + 1
		// Don't read empty strings (1 is empty array of strings)
		if len(strs) < 2 {
			break
		}

		// Temporary map for current step
		var tempMap = map[string]map[string]bool{}

		// Parse strings and add to map
		for _, str := range strs {
			// Cut part without values
			cut := strings.Split(str, " => ")

			// Don't panic if no result
			if cut[0] == "{}" {
				continue
			}

			// TODO: make it independent of keys order returned by Prometheus
			podStartPos := strings.LastIndex(cut[0], `="`)
			podEndPos := strings.LastIndex(cut[0], `"}`)

			namespaceStartPos := strings.Index(cut[0], `="`)
			namespaceEndPos := strings.LastIndex(cut[0], `",`)

			namespace := cut[0][namespaceStartPos+2:namespaceEndPos]
			pod := cut[0][podStartPos+2:podEndPos]

			mapAdd(tempMap, namespace, pod)
		}

		if step == 0 {
			resultMap = tempMap
		}

		// Delete from resultMap values which are not exists in tempMap
		// TODO: understand why it's slow in debug log mode
		for ns, podsMap := range resultMap {
			if step == 0 { break }
			for pod := range podsMap {
				// Try to find pod in
				_, ok := tempMap[ns][pod]
				if !ok {
					delete(resultMap[ns], pod)
					klog.V(5).Infof("Dleteted from resultMap: ns: %v, pod %v\n", ns, pod)
				}
			}
		}
	}

	return resultMap, observedPeriod, nil
}