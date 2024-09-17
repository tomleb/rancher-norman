package generator

var lifecycleTemplate = `package {{.schema.Version.Version}}

import (
	"context"

	{{.importPackage}}
	"k8s.io/apimachinery/pkg/runtime"
	"github.com/rancher/norman/lifecycle"
	"github.com/rancher/norman/resource"
)

type {{.schema.ID}}LifecycleConverter struct {
    lifecycle {{.schema.CodeName}}Lifecycle
}

func (w *{{.schema.ID}}LifecycleConverter) CreateContext(_ context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error) {
    return w.lifecycle.Create(obj)
}

func (w *{{.schema.ID}}LifecycleConverter) RemoveContext(_ context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error) {
    return w.lifecycle.Remove(obj)
}

func (w *{{.schema.ID}}LifecycleConverter) UpdatedContext(_ context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error) {
    return w.lifecycle.Updated(obj)
}

type {{.schema.CodeName}}Lifecycle interface {
	Create(obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
	Remove(obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
	Updated(obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
}

type {{.schema.CodeName}}LifecycleContext interface {
	CreateContext(ctx context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
	RemoveContext(ctx context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
	UpdatedContext(ctx context.Context, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error)
}

type {{.schema.ID}}LifecycleAdapter struct {
	lifecycle {{.schema.CodeName}}LifecycleContext
}

func (w *{{.schema.ID}}LifecycleAdapter) HasCreate() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasCreate()
}

func (w *{{.schema.ID}}LifecycleAdapter) HasFinalize() bool {
	o, ok := w.lifecycle.(lifecycle.ObjectLifecycleCondition)
	return !ok || o.HasFinalize()
}

func (w *{{.schema.ID}}LifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	return w.CreateContext(context.Background(), obj)
}

func (w *{{.schema.ID}}LifecycleAdapter) CreateContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.CreateContext(ctx, obj.(*{{.prefix}}{{.schema.CodeName}}))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *{{.schema.ID}}LifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	return w.FinalizeContext(context.Background(), obj)
}

func (w *{{.schema.ID}}LifecycleAdapter) FinalizeContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.RemoveContext(ctx, obj.(*{{.prefix}}{{.schema.CodeName}}))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *{{.schema.ID}}LifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	return w.UpdatedContext(context.Background(), obj)
}

func (w *{{.schema.ID}}LifecycleAdapter) UpdatedContext(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.UpdatedContext(ctx, obj.(*{{.prefix}}{{.schema.CodeName}}))
	if o == nil {
		return nil, err
	}
	return o, err
}

func New{{.schema.CodeName}}LifecycleAdapter(name string, clusterScoped bool, client {{.schema.CodeName}}Interface, l {{.schema.CodeName}}Lifecycle) {{.schema.CodeName}}HandlerFunc {
	if clusterScoped {
		resource.PutClusterScoped({{.schema.CodeName}}GroupVersionResource)
	}
	adapter := &{{.schema.ID}}LifecycleAdapter{lifecycle: &{{.schema.ID}}LifecycleConverter{lifecycle: l}}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}

func New{{.schema.CodeName}}LifecycleAdapterContext(name string, clusterScoped bool, client {{.schema.CodeName}}Interface, l {{.schema.CodeName}}LifecycleContext) {{.schema.CodeName}}HandlerContextFunc {
	if clusterScoped {
		resource.PutClusterScoped({{.schema.CodeName}}GroupVersionResource)
	}
	adapter := &{{.schema.ID}}LifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapterContext(name, clusterScoped, adapter, client.ObjectClient())
	return func(ctx context.Context, key string, obj *{{.prefix}}{{.schema.CodeName}}) (runtime.Object, error) {
		newObj, err := syncFn(ctx, key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
`
