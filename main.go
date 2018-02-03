package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	"github.com/appscode/go/log"
	"github.com/tamalsaha/go-oneliners"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/api/core/v1"
)

func main() {
	masterURL := ""
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube/config")

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	if err != nil {
		log.Fatalf("Could not get Kubernetes config: %s", err)
	}

	kc := kubernetes.NewForConfigOrDie(config)

	//data, _ := json.MarshalIndent(resourceList, "", "  ")
	//oneliners.FILE(len(data))

	restMapper := NewDefaultRESTMapper([]schema.GroupVersion{})
	oneliners.FILE(restMapper)

	resourceLists, err := kc.Discovery().ServerResources()
	for _, resourceList := range resourceLists {
		for _, resource := range resourceList.APIResources {
			if strings.ContainsRune(resource.Name, '/') {
				continue
			}

			if resource.Kind != "APIService" {
				continue
			}

			fmt.Println(resourceList.GroupVersion, "|_|", resource.Name, "|_|", resource.SingularName, "|_|", resource.Kind)

			gv, _ := schema.ParseGroupVersion(resourceList.GroupVersion)
			plural := gv.WithResource(resource.Name)
			singular := gv.WithResource(resource.SingularName)
			gvk := gv.WithKind(resource.Kind)
			restMapper.AddSpecific(gvk, plural, singular)
		}
	}

	oneliners.FILE(restMapper.kindToPluralResource)
	fmt.Println("__________________________________________________________________________________________________")

	p2 := v1.Pod{}
	fmt.Println(PkgPath(p2))

	// restMapper.ResourceFor()
	p := v1beta1.APIService{}
	fmt.Println(reflect.TypeOf(p).Name())
	fmt.Println(reflect.TypeOf(p).PkgPath())

	fmt.Println(PkgPath(p))

	pp := PkgPath(p)
	if strings.HasPrefix(pp, "k8s.io/") {
		parts := strings.Split(pp, "/")
		if len(parts) < 2 {
		}
		group := parts[len(parts)-2]
		if group == "core" {
			group = ""
		}
		version := parts[len(parts)-1]
		fmt.Println("group = ", group, "   version = ", version)

		rs, err := restMapper.ResourcesFor(schema.GroupVersionKind{Group: group, Version: version, Kind: Kind(p)})
		fmt.Println(err)
		for _, r := range rs {
			fmt.Println("|____ ", r)
		}
	}

	m2, err := meta.Accessor(&p)
	fmt.Println(m2, err)

	// meta.UnsafeGuessKindToResource()
}

func PkgPath(v interface{}) string {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	p := val.Type().PkgPath()
	idx := strings.LastIndex(p, "/vendor/")
	if idx > -1 {
		p = p[idx+len("/vendor/"):]
	}
	return p
}

func Kind(v interface{}) string {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val.Type().Name()
}

type DefaultRESTMapper struct {
	defaultGroupVersions []schema.GroupVersion

	resourceToKind       map[schema.GroupVersionResource]schema.GroupVersionKind
	kindToPluralResource map[schema.GroupVersionKind]schema.GroupVersionResource
	singularToPlural     map[schema.GroupVersionResource]schema.GroupVersionResource
	pluralToSingular     map[schema.GroupVersionResource]schema.GroupVersionResource
}

func NewDefaultRESTMapper(defaultGroupVersions []schema.GroupVersion) *DefaultRESTMapper {
	resourceToKind := make(map[schema.GroupVersionResource]schema.GroupVersionKind)
	kindToPluralResource := make(map[schema.GroupVersionKind]schema.GroupVersionResource)
	singularToPlural := make(map[schema.GroupVersionResource]schema.GroupVersionResource)
	pluralToSingular := make(map[schema.GroupVersionResource]schema.GroupVersionResource)
	// TODO: verify name mappings work correctly when versions differ

	return &DefaultRESTMapper{
		resourceToKind:       resourceToKind,
		kindToPluralResource: kindToPluralResource,
		defaultGroupVersions: defaultGroupVersions,
		singularToPlural:     singularToPlural,
		pluralToSingular:     pluralToSingular,
	}
}

func (m *DefaultRESTMapper) AddSpecific(kind schema.GroupVersionKind, plural, singular schema.GroupVersionResource) {
	m.singularToPlural[singular] = plural
	m.pluralToSingular[plural] = singular

	m.resourceToKind[singular] = kind
	m.resourceToKind[plural] = kind

	m.kindToPluralResource[kind] = plural
}

func (m *DefaultRESTMapper) ResourcesFor(input schema.GroupVersionKind) ([]schema.GroupVersionResource, error) {
	gvk := coerceKindForMatching(input)
	oneliners.FILE(gvk)

	hasKind := len(gvk.Kind) > 0
	hasGroup := len(gvk.Group) > 0
	hasVersion := len(gvk.Version) > 0

	if !hasKind {
		return nil, fmt.Errorf("a kind must be present, got: %v", gvk)
	}

	var ret []schema.GroupVersionResource
	switch {
	case hasGroup:
		// given a group, prefer an exact match.  If you don't find one, resort to a prefix match on group
		foundExactMatch := false
		requestedGroupKind := gvk.GroupKind()
		for kind, plural := range m.kindToPluralResource {
			if kind.GroupKind() == requestedGroupKind && (!hasVersion || kind.Version == gvk.Version) {
				foundExactMatch = true
				ret = append(ret, plural)
			}
		}

		// if you didn't find an exact match, match on group prefixing. This allows storageclass.storage to match
		// storageclass.storage.k8s.io
		if !foundExactMatch {
			for kind, plural := range m.kindToPluralResource {
				if !strings.HasPrefix(kind.Group, requestedGroupKind.Group) {
					continue
				}
				if kind.Kind == requestedGroupKind.Kind && (!hasVersion || kind.Version == gvk.Version) {
					ret = append(ret, plural)
				}
			}
		}

	case hasVersion:
		for kind, plural := range m.kindToPluralResource {
			if kind.Version == gvk.Version && kind.Kind == gvk.Kind {
				oneliners.FILE(kind, "||", plural)
				ret = append(ret, plural)
			}
		}

	default:
		for kind, plural := range m.kindToPluralResource {
			if kind.Kind == gvk.Kind {
				ret = append(ret, plural)
			}
		}
	}

	if len(ret) == 0 {
		return nil, fmt.Errorf("no matches for %v", gvk)
	}

	sort.Sort(resourceByPreferredGroupVersion{ret, m.defaultGroupVersions})
	return ret, nil
}

func (m *DefaultRESTMapper) ResourceFor(input schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	resources, err := m.ResourcesFor(input)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	if len(resources) == 1 {
		return resources[0], nil
	}

	return schema.GroupVersionResource{}, &AmbiguousResourceError{PartialResource: input, MatchingResources: resources}
}

// coerceKindForMatching makes the resource lower case and converts internal versions to unspecified (legacy behavior)
func coerceKindForMatching(gvk schema.GroupVersionKind) schema.GroupVersionKind {
	if gvk.Version == runtime.APIVersionInternal {
		gvk.Version = ""
	}
	return gvk
}

type resourceByPreferredGroupVersion struct {
	list      []schema.GroupVersionResource
	sortOrder []schema.GroupVersion
}

func (o resourceByPreferredGroupVersion) Len() int      { return len(o.list) }
func (o resourceByPreferredGroupVersion) Swap(i, j int) { o.list[i], o.list[j] = o.list[j], o.list[i] }
func (o resourceByPreferredGroupVersion) Less(i, j int) bool {
	lhs := o.list[i]
	rhs := o.list[j]
	if lhs == rhs {
		return false
	}

	if lhs.GroupVersion() == rhs.GroupVersion() {
		return lhs.Resource < rhs.Resource
	}

	// otherwise, the difference is in the GroupVersion, so we need to sort with respect to the preferred order
	lhsIndex := -1
	rhsIndex := -1

	for i := range o.sortOrder {
		if o.sortOrder[i] == lhs.GroupVersion() {
			lhsIndex = i
		}
		if o.sortOrder[i] == rhs.GroupVersion() {
			rhsIndex = i
		}
	}

	if rhsIndex == -1 {
		return true
	}

	return lhsIndex < rhsIndex
}

// AmbiguousResourceError is returned if the RESTMapper finds multiple matches for a resource
type AmbiguousResourceError struct {
	PartialResource schema.GroupVersionKind

	MatchingResources []schema.GroupVersionResource
	MatchingKinds     []schema.GroupVersionKind
}

func (e *AmbiguousResourceError) Error() string {
	switch {
	case len(e.MatchingKinds) > 0 && len(e.MatchingResources) > 0:
		return fmt.Sprintf("%v matches multiple resources %v and kinds %v", e.PartialResource, e.MatchingResources, e.MatchingKinds)
	case len(e.MatchingKinds) > 0:
		return fmt.Sprintf("%v matches multiple kinds %v", e.PartialResource, e.MatchingKinds)
	case len(e.MatchingResources) > 0:
		return fmt.Sprintf("%v matches multiple resources %v", e.PartialResource, e.MatchingResources)
	}
	return fmt.Sprintf("%v matches multiple resources or kinds", e.PartialResource)
}
