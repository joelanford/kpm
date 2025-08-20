package internal

import (
	consolev1 "github.com/openshift/api/console/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	ofv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

var SupportedKinds = sets.New[string](
	// corev1
	"ConfigMap",
	"Secret",
	"Service",
	"ServiceAccount",

	// apiextensionsv1
	"CustomResourceDefinition",

	// rbacv1
	"ClusterRole",
	"ClusterRoleBinding",
	"Role",
	"RoleBinding",

	// ofv1alpha1
	ofv1alpha1.ClusterServiceVersionKind,

	// schedulingv1
	"PriorityClass",

	// policyv1
	"PodDisruptionBudget",

	// autoscalingv1
	"VerticalPodAutoscaler",

	// monitoringv1
	"PrometheusRule",
	"ServiceMonitor",

	// console
	"ConsoleYAMLSample",
	"ConsoleQuickStart",
	"ConsoleCLIDownload",
	"ConsoleLink",
)

func initScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = ofv1alpha1.AddToScheme(scheme)
	_ = schedulingv1.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = autoscalingv1.AddToScheme(scheme)
	_ = monitoringv1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = consolev1.AddToScheme(scheme)
	return scheme
}

var SupportedKindsScheme *runtime.Scheme

func init() {
	SupportedKindsScheme = initScheme()
}
