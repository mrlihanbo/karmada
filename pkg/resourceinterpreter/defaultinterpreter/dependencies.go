package defaultinterpreter

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/karmada-io/karmada/pkg/apis/config/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util"
	"github.com/karmada-io/karmada/pkg/util/helper"
)

type dependenciesInterpreter func(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error)

func getAllDefaultDependenciesInterpreter() map[schema.GroupVersionKind]dependenciesInterpreter {
	s := make(map[schema.GroupVersionKind]dependenciesInterpreter)
	s[appsv1.SchemeGroupVersion.WithKind(util.DeploymentKind)] = getDeploymentDependencies
	s[batchv1.SchemeGroupVersion.WithKind(util.JobKind)] = getJobDependencies
	s[corev1.SchemeGroupVersion.WithKind(util.PodKind)] = getPodDependencies
	s[appsv1.SchemeGroupVersion.WithKind(util.DaemonSetKind)] = getDaemonSetDependencies
	s[appsv1.SchemeGroupVersion.WithKind(util.StatefulSetKind)] = getStatefulSetDependencies
	return s
}

func getDeploymentDependencies(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error) {
	deploymentObj, err := helper.ConvertToDeployment(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Deployment from unstructured object: %v", err)
	}

	podObj, err := GetPodFromTemplate(&deploymentObj.Spec.Template, deploymentObj, nil)
	if err != nil {
		return nil, err
	}

	return getDependenciesFromPodTemplate(cl, podObj)
}

func getJobDependencies(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error) {
	jobObj, err := helper.ConvertToJob(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Job from unstructured object: %v", err)
	}

	podObj, err := GetPodFromTemplate(&jobObj.Spec.Template, jobObj, nil)
	if err != nil {
		return nil, err
	}

	return getDependenciesFromPodTemplate(cl, podObj)
}

func getPodDependencies(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error) {
	podObj, err := helper.ConvertToPod(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Pod from unstructured object: %v", err)
	}

	return getDependenciesFromPodTemplate(cl, podObj)
}

func getDaemonSetDependencies(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error) {
	daemonSetObj, err := helper.ConvertToDaemonSet(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert DaemonSet from unstructured object: %v", err)
	}

	podObj, err := GetPodFromTemplate(&daemonSetObj.Spec.Template, daemonSetObj, nil)
	if err != nil {
		return nil, err
	}

	return getDependenciesFromPodTemplate(cl, podObj)
}

func getStatefulSetDependencies(cl client.Client, object *unstructured.Unstructured) ([]configv1alpha1.DependentObjectReference, error) {
	statefulSetObj, err := helper.ConvertToStatefulSet(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert StatefulSet from unstructured object: %v", err)
	}

	podObj, err := GetPodFromTemplate(&statefulSetObj.Spec.Template, statefulSetObj, nil)
	if err != nil {
		return nil, err
	}

	return getDependenciesFromPodTemplate(cl, podObj)
}

func getDependenciesFromPodTemplate(cl client.Client, podObj *corev1.Pod) ([]configv1alpha1.DependentObjectReference, error) {
	dependentConfigMaps := getConfigMapNames(podObj)
	dependentSecrets := getSecretNames(podObj)
	var dependentObjectRefs []configv1alpha1.DependentObjectReference
	for cm := range dependentConfigMaps {
		dependentObjectRefs = append(dependentObjectRefs, configv1alpha1.DependentObjectReference{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  podObj.Namespace,
			Name:       cm,
		})
	}

	for secret := range dependentSecrets {
		dependentObjectRefs = append(dependentObjectRefs, configv1alpha1.DependentObjectReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Namespace:  podObj.Namespace,
			Name:       secret,
		})
	}

	dependentServices, err := getServiceDependencies(cl, podObj)
	if err != nil {
		return nil, err
	}

	dependentObjectRefs = append(dependentObjectRefs, dependentServices...)
	return dependentObjectRefs, nil
}

func getServiceDependencies(cl client.Client, podObj *corev1.Pod) ([]configv1alpha1.DependentObjectReference, error) {
	serviceList := &corev1.ServiceList{}
	err := cl.List(context.TODO(), serviceList, &client.ListOptions{Namespace: podObj.GetNamespace()})
	if err != nil {
		return nil, err
	}

	dependentServices := getDependentServiceNames(podObj.Labels, serviceList.Items)
	var dependentObjectRef []configv1alpha1.DependentObjectReference
	for service := range dependentServices {
		dependentObjectRef = append(dependentObjectRef, configv1alpha1.DependentObjectReference{
			APIVersion: "v1",
			Kind:       "Service",
			Namespace:  podObj.GetNamespace(),
			Name:       service,
		})
	}

	return dependentObjectRef, nil
}

func getDependentServiceNames(podLabels map[string]string, serviceList []corev1.Service) sets.String {
	dependentServices := sets.String{}
	for _, service := range serviceList {
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			continue
		}

		if service.Spec.Selector == nil {
			// if the service has a nil selector this means selectors match nothing, not everything.
			continue
		}

		if labels.SelectorFromSet(service.Spec.Selector).Matches(labels.Set(podLabels)) {
			dependentServices.Insert(service.Name)
		}
	}
	return dependentServices
}
