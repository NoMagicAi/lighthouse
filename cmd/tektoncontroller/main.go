package main

import (
	"flag"
	"os"

	lighthousev1alpha1 "github.com/jenkins-x/lighthouse/pkg/apis/lighthouse/v1alpha1"
	"github.com/jenkins-x/lighthouse/pkg/clients"
	tektonengine "github.com/jenkins-x/lighthouse/pkg/engines/tekton"
	"github.com/jenkins-x/lighthouse/pkg/interrupts"
	"github.com/jenkins-x/lighthouse/pkg/logrusutil"
	"github.com/sirupsen/logrus"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonversioned "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type options struct {
	namespace               string
	dashboardURL            string
	dashboardTemplate       string
	enableRerunStatusUpdate bool
	kubeAPIQPS              float64
	kubeAPIBurst            int
	maxConcurrentReconciles int
}

func (o *options) Validate() error {
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.namespace, "namespace", "", "The namespace to listen in")
	fs.StringVar(&o.dashboardURL, "dashboard-url", "", "The base URL for the Tekton Dashboard to link to for build reports")
	fs.StringVar(&o.dashboardTemplate, "dashboard-template", "", "The template expression for generating the URL to the build report based on the PipelineRun parameters. If not specified defaults to $LIGHTHOUSE_DASHBOARD_TEMPLATE")
	fs.BoolVar(&o.enableRerunStatusUpdate, "enable-rerun-status-update", false, "Enable updating the status at the git provider when PipelineRuns are rerun")
	fs.Float64Var(&o.kubeAPIQPS, "kube-api-qps", 50, "Maximum QPS to the kube-apiserver from this client")
	fs.IntVar(&o.kubeAPIBurst, "kube-api-burst", 100, "Maximum burst for throttle from this client")
	fs.IntVar(&o.maxConcurrentReconciles, "max-concurrent-reconciles", 4, "Number of LighthouseJobs reconciled concurrently")
	err := fs.Parse(args)
	if err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	return o
}

func main() {
	logrusutil.ComponentInit("lighthouse-tekton-controller")

	scheme := runtime.NewScheme()
	if err := lighthousev1alpha1.AddToScheme(scheme); err != nil {
		logrus.WithError(err).Fatal("Failed to register lighthousev1alpha1 scheme")
	}
	if err := pipelinev1.AddToScheme(scheme); err != nil {
		logrus.WithError(err).Fatal("Failed to register tektoncd-pipelinev1 scheme")
	}

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	cfg, err := clients.GetConfig("", "")
	if err != nil {
		logrus.WithError(err).Fatal("Could not create kubeconfig")
	}
	// client-go defaults to 5 QPS; each job reconcile GETs every TaskRun of its
	// PipelineRun, so after a restart the walk over retained runs took >30 min.
	cfg.QPS = float32(o.kubeAPIQPS)
	cfg.Burst = o.kubeAPIBurst

	mgr, err := ctrl.NewManager(cfg, manager.Options{
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				o.namespace: {},
			},
		},
		Scheme: scheme,
	})
	if err != nil {
		logrus.WithError(err).Fatal("Unable to start manager")
	}

	// Built from cfg so the TaskRun GETs in ConvertPipelineRun share the raised QPS.
	tektonclient, err := tektonversioned.NewForConfig(cfg)
	if err != nil {
		logrus.WithError(err).Fatal(err, "failed to create tekton client")
	}

	lhJobReconciler := tektonengine.NewLighthouseJobReconciler(mgr.GetClient(), mgr.GetAPIReader(), mgr.GetScheme(), tektonclient, o.dashboardURL, o.dashboardTemplate, o.namespace)
	if err = lhJobReconciler.SetupWithManager(mgr, o.maxConcurrentReconciles); err != nil {
		logrus.WithError(err).Fatal("Unable to create controller")
	}

	if o.enableRerunStatusUpdate {
		rerunPipelineRunReconciler := tektonengine.NewRerunPipelineRunReconciler(mgr.GetClient(), mgr.GetScheme())
		if err = rerunPipelineRunReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Fatal("Unable to create RerunPipelineRun controller")
		}
	}

	defer interrupts.WaitForGracefulShutdown()
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("Problem running manager")
	}
}
