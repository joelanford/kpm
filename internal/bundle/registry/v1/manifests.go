package v1

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	consolev1 "github.com/openshift/api/console/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ofv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

const ManifestsDirectory = "manifests/"

type manifests struct {
	fsys          fs.FS
	manifestFiles []manifestFile
}

func (m *manifests) load() error {
	var loadErrs []error
	if err := fs.WalkDir(m.fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		mf, err := newManifestFile(m.fsys, path)
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		m.manifestFiles = append(m.manifestFiles, *mf)
		return nil
	}); err != nil {
		panic("programmer error: walk function should have collected error, not returned it. Error: " + err.Error())
	}
	if err := errors.Join(loadErrs...); err != nil {
		return fmt.Errorf("failed to load manifests: %v", err)
	}
	return nil
}

func (m *manifests) validate() error {
	var validationErrors []error
	for _, validationFn := range []func() error{
		m.validateNoSubDirectories,
		m.validateOneObjectPerFile,
		m.validateExactlyOneCSV,
		m.validateSupportedKinds,
	} {
		if err := validationFn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	if err := errors.Join(validationErrors...); err != nil {
		return fmt.Errorf("invalid registry+v1 manifests: %v", err)
	}
	return nil
}

func (m *manifests) validateNoSubDirectories() error {
	foundSubDirectories := map[string]struct{}{}
	for _, mf := range m.manifestFiles {
		foundSubDirectories[filepath.Dir(mf.filename)] = struct{}{}
	}
	delete(foundSubDirectories, ".")
	if len(foundSubDirectories) == 0 {
		return nil
	}
	return fmt.Errorf("found subdirectories %v: subdirectories not allowed", slices.Sorted(maps.Keys(foundSubDirectories)))
}

func (m *manifests) validateOneObjectPerFile() error {
	var invalidFiles []string
	for _, mf := range m.manifestFiles {
		if len(mf.objects) != 1 {
			invalidFiles = append(invalidFiles, fmt.Sprintf("%q has %d", mf.filename, len(mf.objects)))
		}
	}
	if len(invalidFiles) > 0 {
		return fmt.Errorf("manifest files must contain exactly one object: %v", strings.Join(invalidFiles, ", "))
	}
	return nil
}

func (m *manifests) validateExactlyOneCSV() error {
	totalCount := 0
	foundCSVs := map[string]int{}
	for _, mf := range m.manifestFiles {
		for _, o := range mf.objects {
			if o.GetObjectKind().GroupVersionKind().Kind == ofv1alpha1.ClusterServiceVersionKind {
				totalCount++
				foundCSVs[mf.filename]++
			}
		}
	}
	if totalCount == 0 {
		return fmt.Errorf("exactly one %s object is required, found 0", ofv1alpha1.ClusterServiceVersionKind)
	}
	if totalCount > 1 {
		counts := make([]string, 0, len(foundCSVs))
		for _, filename := range slices.Sorted(maps.Keys(foundCSVs)) {
			csvCount := foundCSVs[filename]
			counts = append(counts, fmt.Sprintf("%q has %d", filename, csvCount))
		}
		return fmt.Errorf("exactly one %s object is required, found %d: %v", ofv1alpha1.ClusterServiceVersionKind, totalCount, strings.Join(counts, ", "))
	}
	return nil
}

var supportedKinds = sets.New[string](
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
	_ = consolev1.AddToScheme(scheme)
	return scheme
}

var supportedKindsScheme *runtime.Scheme

func init() {
	supportedKindsScheme = initScheme()
}

func (m *manifests) validateSupportedKinds() error {
	var unsupported []string
	for _, mf := range m.manifestFiles {
		fileUnsupported := sets.New[string]()
		for _, obj := range mf.objects {
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			if !supportedKinds.Has(kind) {
				fileUnsupported.Insert(kind)
			}
		}
		if len(fileUnsupported) > 0 {
			unsupported = append(unsupported, fmt.Sprintf("file %q contains %v", mf.filename, sets.List(fileUnsupported)))
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("found unsupported kinds: %v", strings.Join(unsupported, ", "))
	}
	return nil
}

type manifestFile struct {
	filename string
	objects  []client.Object
}

func newManifestFile(fsys fs.FS, path string) (*manifestFile, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		m    = &manifestFile{filename: path}
		errs []error
	)
	res := resource.NewLocalBuilder().WithScheme(supportedKindsScheme, supportedKindsScheme.PrioritizedVersionsAllGroups()...).Flatten().Stream(file, path).Do()
	if err := res.Err(); err != nil {
		errs = append(errs, err)
	}
	if err := res.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		m.objects = append(m.objects, info.Object.(client.Object))
		return nil
	}); err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return m, nil
}
