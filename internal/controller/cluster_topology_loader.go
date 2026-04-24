//go:build prof

package controllers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	authorinooperatorv1beta1 "github.com/kuadrant/authorino-operator/api/v1beta1"
	authorinov1beta3 "github.com/kuadrant/authorino/api/v1beta3"
	"github.com/kuadrant/policy-machinery/controller"
	"github.com/kuadrant/policy-machinery/machinery"
	istioclientgonetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	kuadrantv1 "github.com/kuadrant/kuadrant-operator/api/v1"
	kuadrantv1beta1 "github.com/kuadrant/kuadrant-operator/api/v1beta1"
	kuadrantauthorino "github.com/kuadrant/kuadrant-operator/internal/authorino"
)

// ClusterTopologyLoader loads real cluster topology for benchmarking
// Supports both live cluster connection and snapshot files
type ClusterTopologyLoader struct {
	scheme *runtime.Scheme
}

// NewClusterTopologyLoader creates a new loader with all required schemes registered
func NewClusterTopologyLoader() (*ClusterTopologyLoader, error) {
	runtimeScheme := runtime.NewScheme()

	// Register all schemes
	if err := scheme.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add core scheme: %w", err)
	}
	if err := gatewayapiv1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add gateway API scheme: %w", err)
	}
	if err := kuadrantv1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add kuadrant v1 scheme: %w", err)
	}
	if err := kuadrantv1beta1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add kuadrant v1beta1 scheme: %w", err)
	}
	if err := authorinov1beta3.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add authorino scheme: %w", err)
	}
	if err := authorinooperatorv1beta1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add authorino operator scheme: %w", err)
	}
	// Istio schemes are optional
	_ = istioclientgonetworkingv1alpha3.AddToScheme(runtimeScheme)

	return &ClusterTopologyLoader{
		scheme: runtimeScheme,
	}, nil
}

// LoadFromCluster loads topology from a live Kubernetes cluster
func (l *ClusterTopologyLoader) LoadFromCluster(kubeconfigPath, namespace string) (*machinery.Topology, controller.Store, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build config: %w", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: l.scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	ctx := context.Background()
	return l.loadResources(ctx, k8sClient, namespace)
}

// LoadFromSnapshot loads topology from a YAML snapshot file
func (l *ClusterTopologyLoader) LoadFromSnapshot(snapshotPath string) (*machinery.Topology, controller.Store, error) {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	// Create bytes reader for YAML decoder
	reader := bytes.NewReader(data)
	decoder := yaml.NewYAMLOrJSONDecoder(reader, 4096)
	deserializer := serializer.NewCodecFactory(l.scheme).UniversalDeserializer()

	var resources []runtime.Object

	for {
		var raw runtime.RawExtension
		if err := decoder.Decode(&raw); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		if len(raw.Raw) == 0 {
			continue
		}

		obj, _, err := deserializer.Decode(raw.Raw, nil, nil)
		if err != nil {
			// Skip invalid objects
			continue
		}

		resources = append(resources, obj)
	}

	return l.buildTopologyFromResources(resources)
}

// loadResources loads all relevant resources from the cluster
func (l *ClusterTopologyLoader) loadResources(ctx context.Context, k8sClient client.Client, namespace string) (*machinery.Topology, controller.Store, error) {
	var allResources []runtime.Object

	listOpt := l.namespaceListOption(namespace)

	// Kuadrant instances
	kuadrantList := &kuadrantv1beta1.KuadrantList{}
	if err := k8sClient.List(ctx, kuadrantList, listOpt); err == nil {
		for i := range kuadrantList.Items {
			allResources = append(allResources, &kuadrantList.Items[i])
		}
	}

	// GatewayClasses (cluster-scoped, ignore namespace filter)
	gatewayClassList := &gatewayapiv1.GatewayClassList{}
	if err := k8sClient.List(ctx, gatewayClassList); err == nil {
		for i := range gatewayClassList.Items {
			allResources = append(allResources, &gatewayClassList.Items[i])
		}
	}

	// Gateways
	gatewayList := &gatewayapiv1.GatewayList{}
	if err := k8sClient.List(ctx, gatewayList, listOpt); err == nil {
		for i := range gatewayList.Items {
			allResources = append(allResources, &gatewayList.Items[i])
		}
	}

	// HTTPRoutes
	httpRouteList := &gatewayapiv1.HTTPRouteList{}
	if err := k8sClient.List(ctx, httpRouteList, listOpt); err == nil {
		for i := range httpRouteList.Items {
			allResources = append(allResources, &httpRouteList.Items[i])
		}
	}

	// GRPCRoutes
	grpcRouteList := &gatewayapiv1.GRPCRouteList{}
	if err := k8sClient.List(ctx, grpcRouteList, listOpt); err == nil {
		for i := range grpcRouteList.Items {
			allResources = append(allResources, &grpcRouteList.Items[i])
		}
	}

	// AuthPolicies
	authPolicyList := &kuadrantv1.AuthPolicyList{}
	if err := k8sClient.List(ctx, authPolicyList, listOpt); err == nil {
		for i := range authPolicyList.Items {
			allResources = append(allResources, &authPolicyList.Items[i])
		}
	}

	// RateLimitPolicies
	rateLimitPolicyList := &kuadrantv1.RateLimitPolicyList{}
	if err := k8sClient.List(ctx, rateLimitPolicyList, listOpt); err == nil {
		for i := range rateLimitPolicyList.Items {
			allResources = append(allResources, &rateLimitPolicyList.Items[i])
		}
	}

	// AuthConfigs
	authConfigList := &authorinov1beta3.AuthConfigList{}
	if err := k8sClient.List(ctx, authConfigList, listOpt); err == nil {
		for i := range authConfigList.Items {
			allResources = append(allResources, &authConfigList.Items[i])
		}
	}

	// Authorino instances
	authorinoList := &authorinooperatorv1beta1.AuthorinoList{}
	if err := k8sClient.List(ctx, authorinoList, listOpt); err == nil {
		for i := range authorinoList.Items {
			allResources = append(allResources, &authorinoList.Items[i])
		}
	}

	// EnvoyFilters (Istio)
	envoyFilterList := &istioclientgonetworkingv1alpha3.EnvoyFilterList{}
	if err := k8sClient.List(ctx, envoyFilterList, listOpt); err == nil {
		for i := range envoyFilterList.Items {
			allResources = append(allResources, envoyFilterList.Items[i])
		}
	}

	return l.buildTopologyFromResources(allResources)
}

// buildTopologyFromResources constructs a topology from a list of runtime objects
func (l *ClusterTopologyLoader) buildTopologyFromResources(resources []runtime.Object) (*machinery.Topology, controller.Store, error) {
	// Organize resources by type
	var kuadrants []*kuadrantv1beta1.Kuadrant
	var gatewayClasses []*gatewayapiv1.GatewayClass
	var gateways []*gatewayapiv1.Gateway
	var httpRoutes []*gatewayapiv1.HTTPRoute
	var grpcRoutes []*gatewayapiv1.GRPCRoute
	var authPolicies []*kuadrantv1.AuthPolicy
	var rateLimitPolicies []*kuadrantv1.RateLimitPolicy
	var authConfigs []*authorinov1beta3.AuthConfig
	var authorinos []*authorinooperatorv1beta1.Authorino
	var envoyFilters []*istioclientgonetworkingv1alpha3.EnvoyFilter

	for _, res := range resources {
		switch obj := res.(type) {
		case *kuadrantv1beta1.Kuadrant:
			kuadrants = append(kuadrants, obj)
		case *gatewayapiv1.GatewayClass:
			gatewayClasses = append(gatewayClasses, obj)
		case *gatewayapiv1.Gateway:
			gateways = append(gateways, obj)
		case *gatewayapiv1.HTTPRoute:
			httpRoutes = append(httpRoutes, obj)
		case *gatewayapiv1.GRPCRoute:
			grpcRoutes = append(grpcRoutes, obj)
		case *kuadrantv1.AuthPolicy:
			authPolicies = append(authPolicies, obj)
		case *kuadrantv1.RateLimitPolicy:
			rateLimitPolicies = append(rateLimitPolicies, obj)
		case *authorinov1beta3.AuthConfig:
			authConfigs = append(authConfigs, obj)
		case *authorinooperatorv1beta1.Authorino:
			authorinos = append(authorinos, obj)
		case *istioclientgonetworkingv1alpha3.EnvoyFilter:
			envoyFilters = append(envoyFilters, obj)
		}
	}

	// Build store for link functions
	store := make(controller.Store)

	// Add all resources to store
	for _, k := range kuadrants {
		store[string(k.UID)] = k
	}
	for _, gc := range gatewayClasses {
		store[string(gc.UID)] = gc
	}
	for _, gw := range gateways {
		store[string(gw.UID)] = gw
	}
	for _, hr := range httpRoutes {
		store[string(hr.UID)] = hr
	}
	for _, gr := range grpcRoutes {
		store[string(gr.UID)] = gr
	}
	for _, ap := range authPolicies {
		store[string(ap.UID)] = ap
	}
	for _, rlp := range rateLimitPolicies {
		store[string(rlp.UID)] = rlp
	}
	for _, ac := range authConfigs {
		authConfigRuntimeObj := &controller.RuntimeObject{Object: ac}
		store[string(ac.UID)] = authConfigRuntimeObj
	}
	for _, auth := range authorinos {
		authorinoRuntimeObj := &controller.RuntimeObject{Object: auth}
		store[string(auth.UID)] = authorinoRuntimeObj
	}
	for _, ef := range envoyFilters {
		envoyFilterRuntimeObj := &controller.RuntimeObject{Object: ef}
		store[string(ef.UID)] = envoyFilterRuntimeObj
	}

	// Convert to machinery-compatible slices
	kuadrantObjs := make([]machinery.Object, 0, len(kuadrants)+len(authorinos)+len(authConfigs)+len(envoyFilters))
	for _, k := range kuadrants {
		kuadrantObjs = append(kuadrantObjs, k)
	}
	for _, auth := range authorinos {
		kuadrantObjs = append(kuadrantObjs, store[string(auth.UID)].(machinery.Object))
	}
	for _, ac := range authConfigs {
		kuadrantObjs = append(kuadrantObjs, store[string(ac.UID)].(machinery.Object))
	}
	for _, ef := range envoyFilters {
		kuadrantObjs = append(kuadrantObjs, store[string(ef.UID)].(machinery.Object))
	}

	policies := make([]machinery.Policy, 0, len(authPolicies)+len(rateLimitPolicies))
	for _, ap := range authPolicies {
		policies = append(policies, ap)
	}
	for _, rlp := range rateLimitPolicies {
		policies = append(policies, rlp)
	}

	// Build topology with link functions
	topologyOpts := []machinery.GatewayAPITopologyOptionsFunc{
		machinery.WithGatewayClasses(gatewayClasses...),
		machinery.WithGateways(gateways...),
		machinery.ExpandGatewayListeners(),
		machinery.WithGatewayAPITopologyPolicies(policies...),
		machinery.WithGatewayAPITopologyObjects(kuadrantObjs...),
		machinery.WithGatewayAPITopologyLinks(
			kuadrantv1beta1.LinkKuadrantToGatewayClasses(store),
			kuadrantauthorino.LinkHTTPRouteRuleToAuthConfig(store),
			kuadrantauthorino.LinkGRPCRouteRuleToAuthConfig(store),
		),
	}

	if len(httpRoutes) > 0 {
		topologyOpts = append(topologyOpts,
			machinery.WithHTTPRoutes(httpRoutes...),
			machinery.ExpandHTTPRouteRules(),
		)
	}

	if len(grpcRoutes) > 0 {
		topologyOpts = append(topologyOpts,
			machinery.WithGRPCRoutes(grpcRoutes...),
			machinery.ExpandGRPCRouteRules(),
		)
	}

	topology, err := machinery.NewGatewayAPITopology(topologyOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build topology: %w", err)
	}

	return topology, store, nil
}

func (l *ClusterTopologyLoader) namespaceListOption(namespace string) client.ListOption {
	if namespace == "" {
		return client.InNamespace(corev1.NamespaceAll)
	}
	return client.InNamespace(namespace)
}

// GetDefaultKubeconfigPath returns the default kubeconfig path
func GetDefaultKubeconfigPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "config")
	}
	return ""
}
