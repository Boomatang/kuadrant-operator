package kuadranttools

import (
	"fmt"
	"github.com/kuadrant/kuadrant-operator/api/v1beta1"
	limitadorv1alpha1 "github.com/kuadrant/limitador-operator/api/v1alpha1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LimitadorMutator(existingObj, desiredObj client.Object) (bool, error) {
	update := false
	existing, ok := existingObj.(*limitadorv1alpha1.Limitador)
	if !ok {
		return false, fmt.Errorf("%T is not a *limitadorv1alpha1.Limitador", existingObj)
	}
	desired, ok := desiredObj.(*limitadorv1alpha1.Limitador)
	if !ok {
		return false, fmt.Errorf("%T is not a *limitadorv1alpha1.Limitador", existingObj)
	}

	existingSpec := limitadorSpecSubSet(existing.Spec)
	desiredSpec := limitadorSpecSubSet(desired.Spec)

	if !reflect.DeepEqual(existingSpec, desiredSpec) {
		update = true
		existing.Spec.Affinity = desired.Spec.Affinity
		existing.Spec.PodDisruptionBudget = desired.Spec.PodDisruptionBudget
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.ResourceRequirements = desired.Spec.ResourceRequirements
		existing.Spec.Storage = desired.Spec.Storage
	}

	return update, nil
}

func limitadorSpecSubSet(spec limitadorv1alpha1.LimitadorSpec) v1beta1.LimitadorSpec {
	out := v1beta1.LimitadorSpec{}

	out.Affinity = spec.Affinity
	out.PodDisruptionBudget = spec.PodDisruptionBudget
	out.Replicas = spec.Replicas
	out.ResourceRequirements = spec.ResourceRequirements
	out.Storage = spec.Storage

	return out
}