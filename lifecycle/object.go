package lifecycle

import (
	"context"
	"fmt"
	"reflect"

	"github.com/rancher/norman/objectclient"
	"github.com/rancher/norman/types/slice"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	created            = "lifecycle.cattle.io/create"
	finalizerKey       = "controller.cattle.io/"
	ScopedFinalizerKey = "clusterscoped.controller.cattle.io/"
)

type ObjectLifecycle interface {
	Create(obj runtime.Object) (runtime.Object, error)
	Finalize(obj runtime.Object) (runtime.Object, error)
	Updated(obj runtime.Object) (runtime.Object, error)
}

type ObjectLifecycleContext interface {
	CreateContext(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	FinalizeContext(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	UpdatedContext(ctx context.Context, obj runtime.Object) (runtime.Object, error)
}

type ObjectLifecycleCondition interface {
	HasCreate() bool
	HasFinalize() bool
}

type objectLifecycleAdapter struct {
	name          string
	clusterScoped bool
	lifecycle     ObjectLifecycleContext
	objectClient  *objectclient.ObjectClient
}

var _ ObjectLifecycleContext = (*ObjectLifecycleContextFuncs)(nil)

type ObjectLifecycleContextFuncs struct {
	CreateContextFunc   func(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	FinalizeContextFunc func(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	UpdatedContextFunc  func(ctx context.Context, obj runtime.Object) (runtime.Object, error)
}

func (o *ObjectLifecycleContextFuncs) CreateContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	return o.CreateContextFunc(ctx, obj)
}

func (o *ObjectLifecycleContextFuncs) FinalizeContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	return o.FinalizeContextFunc(ctx, obj)
}

func (o *ObjectLifecycleContextFuncs) UpdatedContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	return o.UpdatedContextFunc(ctx, obj)
}

func ToContext(lifecycle ObjectLifecycle) ObjectLifecycleContext {
	return &ObjectLifecycleContextFuncs{
		CreateContextFunc: func(_ context.Context, obj runtime.Object) (runtime.Object, error) {
			return lifecycle.Create(obj)
		},
		UpdatedContextFunc: func(_ context.Context, obj runtime.Object) (runtime.Object, error) {
			return lifecycle.Updated(obj)
		},
		FinalizeContextFunc: func(_ context.Context, obj runtime.Object) (runtime.Object, error) {
			return lifecycle.Finalize(obj)
		},
	}
}

func NewObjectLifecycleAdapter(name string, clusterScoped bool, lifecycle ObjectLifecycle, objectClient *objectclient.ObjectClient) func(key string, obj interface{}) (interface{}, error) {
	o := objectLifecycleAdapter{
		name:          name,
		clusterScoped: clusterScoped,
		lifecycle:     ToContext(lifecycle),
		objectClient:  objectClient,
	}
	return o.sync
}

func NewObjectLifecycleAdapterContext(name string, clusterScoped bool, lifecycle ObjectLifecycleContext, objectClient *objectclient.ObjectClient) func(ctx context.Context, key string, obj interface{}) (interface{}, error) {
	o := objectLifecycleAdapter{
		name:          name,
		clusterScoped: clusterScoped,
		lifecycle:     lifecycle,
		objectClient:  objectClient,
	}
	return o.syncContext
}

func (o *objectLifecycleAdapter) sync(key string, in interface{}) (interface{}, error) {
	return o.syncContext(context.Background(), key, in)
}

func (o *objectLifecycleAdapter) syncContext(ctx context.Context, key string, in interface{}) (interface{}, error) {
	if in == nil || reflect.ValueOf(in).IsNil() {
		return nil, nil
	}

	obj, ok := in.(runtime.Object)
	if !ok {
		return nil, nil
	}

	if newObj, cont, err := o.finalize(ctx, obj); err != nil || !cont {
		return nil, err
	} else if newObj != nil {
		obj = newObj
	}

	if newObj, cont, err := o.create(ctx, obj); err != nil || !cont {
		return nil, err
	} else if newObj != nil {
		obj = newObj
	}

	return o.record(ctx, obj, o.lifecycle.UpdatedContext)
}

func (o *objectLifecycleAdapter) update(name string, orig, obj runtime.Object) (runtime.Object, error) {
	if obj != nil && orig != nil && !reflect.DeepEqual(orig, obj) {
		newObj, err := o.objectClient.Update(name, obj)
		if newObj != nil {
			return newObj, err
		}
		return obj, err
	}
	if obj == nil {
		return orig, nil
	}
	return obj, nil
}

func (o *objectLifecycleAdapter) finalize(ctx context.Context, obj runtime.Object) (runtime.Object, bool, error) {
	if !o.hasFinalize() {
		return obj, true, nil
	}

	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, false, err
	}

	// Check finalize
	if metadata.GetDeletionTimestamp() == nil {
		return nil, true, nil
	}

	if !slice.ContainsString(metadata.GetFinalizers(), o.constructFinalizerKey()) {
		return nil, false, nil
	}

	newObj, err := o.record(ctx, obj, o.lifecycle.FinalizeContext)
	if err != nil {
		return obj, false, err
	}

	obj, err = o.removeFinalizer(o.constructFinalizerKey(), maybeDeepCopy(obj, newObj))
	return obj, false, err
}

func maybeDeepCopy(old, newObj runtime.Object) runtime.Object {
	if old == newObj {
		return old.DeepCopyObject()
	}
	return newObj
}

func (o *objectLifecycleAdapter) removeFinalizer(name string, obj runtime.Object) (runtime.Object, error) {
	for i := 0; i < 3; i++ {
		metadata, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}

		var finalizers []string
		for _, finalizer := range metadata.GetFinalizers() {
			if finalizer == name {
				continue
			}
			finalizers = append(finalizers, finalizer)
		}
		metadata.SetFinalizers(finalizers)

		newObj, err := o.objectClient.Update(metadata.GetName(), obj)
		if err == nil {
			return newObj, nil
		}

		obj, err = o.objectClient.GetNamespaced(metadata.GetNamespace(), metadata.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("failed to remove finalizer on %s", name)
}

func (o *objectLifecycleAdapter) createKey() string {
	return created + "." + o.name
}

func (o *objectLifecycleAdapter) constructFinalizerKey() string {
	if o.clusterScoped {
		return ScopedFinalizerKey + o.name
	}
	return finalizerKey + o.name
}

func (o *objectLifecycleAdapter) hasFinalize() bool {
	cond, ok := o.lifecycle.(ObjectLifecycleCondition)
	return !ok || cond.HasFinalize()
}

func (o *objectLifecycleAdapter) hasCreate() bool {
	cond, ok := o.lifecycle.(ObjectLifecycleCondition)
	return !ok || cond.HasCreate()
}

func (o *objectLifecycleAdapter) record(ctx context.Context, obj runtime.Object, f func(context.Context, runtime.Object) (runtime.Object, error)) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, err
	}

	origObj := obj
	obj = origObj.DeepCopyObject()
	if newObj, err := checkNil(ctx, obj, f); err != nil {
		newObj, _ = o.update(metadata.GetName(), origObj, newObj)
		return newObj, err
	} else if newObj != nil {
		return o.update(metadata.GetName(), origObj, newObj)
	}
	return obj, nil
}

func checkNil(ctx context.Context, obj runtime.Object, f func(context.Context, runtime.Object) (runtime.Object, error)) (runtime.Object, error) {
	obj, err := f(ctx, obj)
	if obj == nil || reflect.ValueOf(obj).IsNil() {
		return nil, err
	}
	return obj, err
}

func (o *objectLifecycleAdapter) create(ctx context.Context, obj runtime.Object) (runtime.Object, bool, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return obj, false, err
	}

	if o.isInitialized(metadata) {
		return nil, true, nil
	}

	if o.hasFinalize() {
		obj, err = o.addFinalizer(obj)
		if err != nil {
			return obj, false, err
		}
	}

	if !o.hasCreate() {
		return obj, true, err
	}

	obj, err = o.record(ctx, obj, o.lifecycle.CreateContext)
	if err != nil {
		return obj, false, err
	}

	obj, err = o.setInitialized(obj)
	return obj, false, err
}

func (o *objectLifecycleAdapter) isInitialized(metadata metav1.Object) bool {
	initialized := o.createKey()
	return metadata.GetAnnotations()[initialized] == "true"
}

func (o *objectLifecycleAdapter) setInitialized(obj runtime.Object) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	initialized := o.createKey()

	if metadata.GetAnnotations() == nil {
		metadata.SetAnnotations(map[string]string{})
	}
	metadata.GetAnnotations()[initialized] = "true"

	return o.objectClient.Update(metadata.GetName(), obj)
}

func (o *objectLifecycleAdapter) addFinalizer(obj runtime.Object) (runtime.Object, error) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	if slice.ContainsString(metadata.GetFinalizers(), o.constructFinalizerKey()) {
		return obj, nil
	}

	obj = obj.DeepCopyObject()
	metadata, err = meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	metadata.SetFinalizers(append(metadata.GetFinalizers(), o.constructFinalizerKey()))
	return o.objectClient.Update(metadata.GetName(), obj)
}
