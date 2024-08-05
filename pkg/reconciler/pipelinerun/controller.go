package pipelinerun

import (
	"context"
	"os"

	"github.com/openshift-pipelines/tektoncd-pruner/pkg/reconciler/helper"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	pipelineruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1/pipelinerun"
	pipelinerunreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1/pipelinerun"
	"go.uber.org/zap"
	"k8s.io/utils/clock"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController creates a Reconciler and returns the result of NewImpl.
func NewController(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	// Obtain an informer to both the main and child resources. These will be started by
	// the injection framework automatically. They'll keep a cached representation of the
	// cluster's state of the respective resource at all times.
	pipelineRunInformer := pipelineruninformer.Get(ctx)

	logger := logging.FromContext(ctx)

	pipelineRunFuncs := &PipelineRunFuncs{
		client: pipelineclient.Get(ctx),
	}
	ttlHandler, err := helper.NewTTLHandler(clock.RealClock{}, pipelineRunFuncs)
	if err != nil {
		logger.Fatal("error on getting ttl handler", zap.Error(err))
	}

	historyLimiter, err := helper.NewHistoryLimiter(pipelineRunFuncs)
	if err != nil {
		logger.Fatal("error on getting history limiter", zap.Error(err))
	}

	r := &Reconciler{
		// The client will be needed to create/delete Pods via the API.
		kubeclient:     kubeclient.Get(ctx),
		ttlHandler:     ttlHandler,
		historyLimiter: historyLimiter,
	}

	// number of works to process the events
	concurrentWorkers, err := helper.GetEnvValueAsInt(helper.EnvTTLConcurrentWorkersPipelineRun, helper.DefaultTTLConcurrentWorkersPipelineRun)
	if err != nil {
		logger.Fatalw("error on getting PipelineRun ttl concurrent workers count",
			"environmentKey", helper.EnvTTLConcurrentWorkersPipelineRun, "environmentValue", os.Getenv(helper.EnvTTLConcurrentWorkersPipelineRun),
			zap.Error(err),
		)
	}

	ctrlOptions := controller.Options{
		Concurrency: concurrentWorkers,
	}

	impl := pipelinerunreconciler.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options { return ctrlOptions })

	// listen for events on the main resource and enqueue themselves.
	pipelineRunInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))
	return impl
}
