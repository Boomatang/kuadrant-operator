package controllers

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kuadrant/policy-machinery/controller"
	"github.com/kuadrant/policy-machinery/machinery"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	kuadrantv1beta1 "github.com/kuadrant/kuadrant-operator/api/v1beta1"
	kuadrantauthorino "github.com/kuadrant/kuadrant-operator/internal/authorino"
)

const authConfigFinalizer = "kuadrant.io/authconfigs"

type FinalizerMaintianerReconciler struct {
	client *dynamic.DynamicClient
}

func NewFinalizerMaintianerReconciler(client *dynamic.DynamicClient) *FinalizerMaintianerReconciler {
	return &FinalizerMaintianerReconciler{client: client}
}

func (r *FinalizerMaintianerReconciler) Subscription() *controller.Subscription {
	return &controller.Subscription{
		Events: []controller.ResourceEventMatcher{
			{Kind: &kuadrantauthorino.AuthConfigGroupKind},
			{Kind: &kuadrantv1beta1.KuadrantGroupKind},
		},
		ReconcileFunc: r.reconcile,
	}
}

func (r *FinalizerMaintianerReconciler) reconcile(ctx context.Context, _ []controller.ResourceEvent, topology *machinery.Topology, _ error, s *sync.Map) error {
	logger := controller.LoggerFromContext(ctx).WithName("FinalizerMaintianerReconciler").WithName("reconcile").WithValues("context", ctx)

	logger.Error(fmt.Errorf("testing"), "You are here")
	kuadrant := GetKuadrantFromTopology(topology, s)
	current := kuadrant.GetFinalizers()

	modified := false

	// get resource type from topology
	logger.V(1).Info("get list of authconfigs.")
	authConfigs := topology.Policies().Objects().Items(func(o machinery.Object) bool {
		return o.GroupVersionKind().Kind == kuadrantauthorino.AuthConfigGroupKind.Kind
	})
	if len(authConfigs) > 0 {
		// add finalizer
		logger.V(1).Info("add finalizer")
		if !slices.Contains(current, authConfigFinalizer) {
			current = append(current, authConfigFinalizer)
			modified = true
		}

	} else {
		// remove finalizer
		logger.V(1).Info("removal finalizer")
		idx := slices.Index(current, authConfigFinalizer)
		if idx >= 0 {
			current = slices.Delete(current, idx, idx+1)
			modified = true
		}
	}

	// END: get resource type from topology

	// Repeat: get resource type from topology as needed

	if modified {
		kuadrant.SetFinalizers(current)
		err := r.updateKuadrant(ctx, kuadrant)
		logger.Info("Kuadrant updated", "error", err)
	}

	return nil
}

func (r *FinalizerMaintianerReconciler) updateKuadrant(ctx context.Context, kObj *kuadrantv1beta1.Kuadrant) error {
	obj, err := controller.Destruct(kObj)
	if err != nil {
		return err
	}
	_, err = r.client.Resource(kuadrantv1beta1.KuadrantsResource).Namespace(kObj.GetNamespace()).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}
