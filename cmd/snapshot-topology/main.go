//go:build prof

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	authorinooperatorv1beta1 "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinov1beta3 "github.com/kuadrant/authorino/api/v1beta3"
	istioclientgonetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	kuadrantv1 "github.com/kuadrant/kuadrant-operator/api/v1"
	kuadrantv1beta1 "github.com/kuadrant/kuadrant-operator/api/v1beta1"
)

// SnapshotTopology exports the current cluster topology to a YAML file
// that can be loaded by benchmarks for repeatable testing.
//
// Build:
//   go build -tags=prof -o bin/snapshot-topology ./cmd/snapshot-topology
//
// Usage:
//   ./bin/snapshot-topology -output=cluster-snapshot.yaml
//   ./bin/snapshot-topology -config=$HOME/.kube/config -output=snapshot.yaml
//
// The snapshot includes all resources relevant to AuthPolicy evaluation:
// - Kuadrant
// - GatewayClasses
// - Gateways
// - HTTPRoutes
// - GRPCRoutes
// - AuthPolicies
// - AuthConfigs
// - Authorino
// - EnvoyFilters
// - WasmPlugins

func main() {
	var kubeconfigPath string
	var output string

	// Determine default kubeconfig path
	defaultKubeconfig := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeconfig = filepath.Join(home, ".kube", "config")
	}

	flag.StringVar(&kubeconfigPath, "config", defaultKubeconfig, "(optional) absolute path to the kubeconfig file")
	flag.StringVar(&output, "output", "cluster-snapshot.yaml", "output file path for the snapshot")
	flag.Parse()

	if err := run(kubeconfigPath, output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(kubeconfigPath, outputPath string) error {
	// Build k8s client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create scheme with all required types
	runtimeScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add core scheme: %w", err)
	}
	if err := gatewayapiv1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add gateway API scheme: %w", err)
	}
	if err := kuadrantv1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add kuadrant v1 scheme: %w", err)
	}
	if err := kuadrantv1beta1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add kuadrant v1beta1 scheme: %w", err)
	}
	if err := authorinov1beta3.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add authorino scheme: %w", err)
	}
	if err := authorinooperatorv1beta1.AddToScheme(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add authorino operator scheme: %w", err)
	}
	if err := istioclientgonetworkingv1alpha3.AddToScheme(runtimeScheme); err != nil {
		// Istio might not be installed, log but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to add Istio scheme (might not be installed): %v\n", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: runtimeScheme})
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	ctx := context.Background()

	// Collect all resources
	var allResources []runtime.Object

	// Kuadrant instances
	kuadrantList := &kuadrantv1beta1.KuadrantList{}
	if err := k8sClient.List(ctx, kuadrantList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list Kuadrant resources: %v\n", err)
	} else {
		for i := range kuadrantList.Items {
			allResources = append(allResources, &kuadrantList.Items[i])
		}
		fmt.Printf("Found %d Kuadrant instance(s)\n", len(kuadrantList.Items))
	}

	// GatewayClasses (cluster-scoped)
	gatewayClassList := &gatewayapiv1.GatewayClassList{}
	if err := k8sClient.List(ctx, gatewayClassList); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list GatewayClasses: %v\n", err)
	} else {
		for i := range gatewayClassList.Items {
			allResources = append(allResources, &gatewayClassList.Items[i])
		}
		fmt.Printf("Found %d GatewayClass(es)\n", len(gatewayClassList.Items))
	}

	// Gateways
	gatewayList := &gatewayapiv1.GatewayList{}
	if err := k8sClient.List(ctx, gatewayList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list Gateways: %v\n", err)
	} else {
		for i := range gatewayList.Items {
			allResources = append(allResources, &gatewayList.Items[i])
		}
		fmt.Printf("Found %d Gateway(s)\n", len(gatewayList.Items))
	}

	// HTTPRoutes
	httpRouteList := &gatewayapiv1.HTTPRouteList{}
	if err := k8sClient.List(ctx, httpRouteList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list HTTPRoutes: %v\n", err)
	} else {
		for i := range httpRouteList.Items {
			allResources = append(allResources, &httpRouteList.Items[i])
		}
		fmt.Printf("Found %d HTTPRoute(s)\n", len(httpRouteList.Items))
	}

	// GRPCRoutes
	grpcRouteList := &gatewayapiv1.GRPCRouteList{}
	if err := k8sClient.List(ctx, grpcRouteList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list GRPCRoutes: %v\n", err)
	} else {
		for i := range grpcRouteList.Items {
			allResources = append(allResources, &grpcRouteList.Items[i])
		}
		fmt.Printf("Found %d GRPCRoute(s)\n", len(grpcRouteList.Items))
	}

	// AuthPolicies
	authPolicyList := &kuadrantv1.AuthPolicyList{}
	if err := k8sClient.List(ctx, authPolicyList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list AuthPolicies: %v\n", err)
	} else {
		for i := range authPolicyList.Items {
			allResources = append(allResources, &authPolicyList.Items[i])
		}
		fmt.Printf("Found %d AuthPolicies\n", len(authPolicyList.Items))
	}

	// RateLimitPolicies
	rateLimitPolicyList := &kuadrantv1.RateLimitPolicyList{}
	if err := k8sClient.List(ctx, rateLimitPolicyList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list RateLimitPolicies: %v\n", err)
	} else {
		for i := range rateLimitPolicyList.Items {
			allResources = append(allResources, &rateLimitPolicyList.Items[i])
		}
		fmt.Printf("Found %d RateLimitPolicies\n", len(rateLimitPolicyList.Items))
	}

	// AuthConfigs
	authConfigList := &authorinov1beta3.AuthConfigList{}
	if err := k8sClient.List(ctx, authConfigList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list AuthConfigs: %v\n", err)
	} else {
		for i := range authConfigList.Items {
			allResources = append(allResources, &authConfigList.Items[i])
		}
		fmt.Printf("Found %d AuthConfig(s)\n", len(authConfigList.Items))
	}

	// Authorino instances
	authorinoList := &authorinooperatorv1beta1.AuthorinoList{}
	if err := k8sClient.List(ctx, authorinoList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list Authorino instances: %v\n", err)
	} else {
		for i := range authorinoList.Items {
			allResources = append(allResources, &authorinoList.Items[i])
		}
		fmt.Printf("Found %d Authorino instance(s)\n", len(authorinoList.Items))
	}

	// EnvoyFilters (Istio)
	envoyFilterList := &istioclientgonetworkingv1alpha3.EnvoyFilterList{}
	if err := k8sClient.List(ctx, envoyFilterList, client.InNamespace(corev1.NamespaceAll)); err != nil {
		// Istio might not be installed
		fmt.Fprintf(os.Stderr, "Info: Istio not available or no EnvoyFilters found: %v\n", err)
	} else {
		for i := range envoyFilterList.Items {
			allResources = append(allResources, envoyFilterList.Items[i])
		}
		fmt.Printf("Found %d EnvoyFilter(s)\n", len(envoyFilterList.Items))
	}

	// WasmPlugins (Istio)
	// Note: WasmPlugin type import would be needed here
	// For now, we skip WasmPlugins in snapshot as they're optional

	fmt.Printf("\nTotal resources collected: %d\n", len(allResources))

	// Write to YAML file
	if err := writeSnapshotToYAML(allResources, outputPath, runtimeScheme); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	fmt.Printf("Snapshot written to: %s\n", outputPath)
	return nil
}

func writeSnapshotToYAML(resources []runtime.Object, outputPath string, runtimeScheme *runtime.Scheme) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	// Create YAML serializer
	codecFactory := serializer.NewCodecFactory(runtimeScheme)
	yamlSerializer := codecFactory.LegacyCodec(runtimeScheme.PrioritizedVersionsAllGroups()...)

	// Write header
	fmt.Fprintf(f, "# Kuadrant Cluster Topology Snapshot\n")
	fmt.Fprintf(f, "# Generated: %s\n", metav1.Now().String())
	fmt.Fprintf(f, "# Total resources: %d\n", len(resources))
	fmt.Fprintf(f, "---\n")

	// Serialize each resource
	for i, resource := range resources {
		// Clear managed fields and resource version for cleaner snapshots
		if obj, ok := resource.(client.Object); ok {
			obj.SetManagedFields(nil)
			obj.SetResourceVersion("")
		}

		data, err := runtime.Encode(yamlSerializer, resource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to encode resource %d: %v\n", i, err)
			continue
		}

		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("failed to write resource %d: %w", i, err)
		}

		// Add separator between resources
		if i < len(resources)-1 {
			fmt.Fprintf(f, "---\n")
		}
	}

	return nil
}
