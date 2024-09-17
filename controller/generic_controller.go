package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/lasso/pkg/controller"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type HandlerFunc func(key string, obj interface{}) (interface{}, error)

type HandlerContextFunc func(ctx context.Context, key string, obj interface{}) (interface{}, error)

type GenericController interface {
	Informer() cache.SharedIndexInformer
	AddHandler(ctx context.Context, name string, handler HandlerFunc)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, after time.Duration)
}

type GenericControllerContext interface {
	AddHandlerContext(ctx context.Context, name string, handler HandlerContextFunc) error
}

type genericController struct {
	controller controller.SharedController
	informer   cache.SharedIndexInformer
	name       string
	namespace  string
}

var _ GenericControllerContext = (*genericController)(nil)

func NewGenericController(namespace, name string, controller controller.SharedController) GenericController {
	return &genericController{
		controller: controller,
		informer:   controller.Informer(),
		name:       name,
		namespace:  namespace,
	}
}

func (g *genericController) Informer() cache.SharedIndexInformer {
	return g.informer
}

func (g *genericController) Enqueue(namespace, name string) {
	g.controller.Enqueue(namespace, name)
}

func (g *genericController) EnqueueAfter(namespace, name string, after time.Duration) {
	g.controller.EnqueueAfter(namespace, name, after)
}

func (g *genericController) AddHandlerContext(ctx context.Context, name string, handler HandlerContextFunc) error {
	controllerCtx, ok := g.controller.(controller.SharedControllerContext)
	if !ok {
		return fmt.Errorf("controller not a SharedControllerContext")
	}
	controllerCtx.RegisterHandlerContext(ctx, name, controller.SharedControllerHandlerContextFunc(func(ctx context.Context, key string, obj runtime.Object) (runtime.Object, error) {
		if !isNamespace(g.namespace, obj) {
			return obj, nil
		}
		logrus.Tracef("%s calling handler %s %s", g.name, name, key)
		result, err := handler(ctx, key, obj)
		runtimeObject, _ := result.(runtime.Object)
		if _, ok := err.(*ForgetError); ok {
			logrus.Tracef("%v %v completed with dropped err: %v", g.name, key, err)
			return runtimeObject, controller.ErrIgnore
		}
		return runtimeObject, err
	}))
	return nil
}

func (g *genericController) AddHandler(ctx context.Context, name string, handler HandlerFunc) {
	g.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(func(key string, obj runtime.Object) (runtime.Object, error) {
		if !isNamespace(g.namespace, obj) {
			return obj, nil
		}
		logrus.Tracef("%s calling handler %s %s", g.name, name, key)
		result, err := handler(key, obj)
		runtimeObject, _ := result.(runtime.Object)
		if _, ok := err.(*ForgetError); ok {
			logrus.Tracef("%v %v completed with dropped err: %v", g.name, key, err)
			return runtimeObject, controller.ErrIgnore
		}
		return runtimeObject, err
	}))
}

func isNamespace(namespace string, obj runtime.Object) bool {
	if namespace == "" || obj == nil {
		return true
	}
	meta, err := meta.Accessor(obj)
	if err != nil {
		// if you can't figure out the namespace, just let it through
		return true
	}
	return meta.GetNamespace() == namespace
}
