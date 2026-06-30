package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sandboxv1 "sandbox-operator/api/v1alpha1"
	"sandbox-operator/internal/controller"
	"sandbox-operator/internal/credentials"
	"sandbox-operator/internal/openapi"
	"sandbox-operator/internal/operation"
	sandboxwebhook "sandbox-operator/internal/webhook"
)

var scheme = clientgoscheme.Scheme

func init() {
	utilruntime.Must(sandboxv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var openapiBaseURL string
	var openapiService string
	var openapiVersion string
	var openapiAuthMode string
	var defaultOpenAPISecretName string
	var enableLeaderElection bool
	var pollInterval time.Duration
	var pollPageSize int
	var maxConcurrentNamespaces int

	flag.StringVar(&metricsAddr, "metrics-bind-address", envOrDefault("METRICS_BIND_ADDRESS", ":8080"), "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", envOrDefault("HEALTH_PROBE_BIND_ADDRESS", ":8081"), "The address the probe endpoint binds to.")
	flag.StringVar(&openapiBaseURL, "openapi-base-url", envOrDefault("OPENAPI_BASE_URL", "http://aicp.cn-beijing-6.inner.api.ksyun.com"), "Sandbox OpenAPI base URL.")
	flag.StringVar(&openapiService, "openapi-service", envOrDefault("OPENAPI_SERVICE", "aicp"), "Sandbox OpenAPI KOP service name.")
	flag.StringVar(&openapiVersion, "openapi-version", envOrDefault("OPENAPI_VERSION", "2026-04-01"), "Sandbox OpenAPI version.")
	flag.StringVar(&openapiAuthMode, "openapi-auth-mode", envOrDefault("OPENAPI_AUTH_MODE", "kop-sigv4"), "OpenAPI auth mode: direct or kop-sigv4.")
	flag.StringVar(&defaultOpenAPISecretName, "default-openapi-credential-secret", envOrDefault("DEFAULT_OPENAPI_CREDENTIAL_SECRET", credentials.DefaultOpenAPISecretName), "Default Secret name for OpenAPI AK/SK in each business namespace.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", envBoolOrDefault("LEADER_ELECT", false), "Enable leader election for controller manager.")
	flag.DurationVar(&pollInterval, "poll-interval", envDurationOrDefault("POLL_INTERVAL", 30*time.Second), "OpenAPI polling interval.")
	flag.IntVar(&pollPageSize, "poll-page-size", envIntOrDefault("POLL_PAGE_SIZE", 100), "OpenAPI list page size. The OpenAPI accepts values from 1 to 100.")
	flag.IntVar(&maxConcurrentNamespaces, "max-concurrent-namespaces", envIntOrDefault("MAX_CONCURRENT_NAMESPACES", 5), "Maximum namespaces to sync concurrently.")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "sandbox-operator.sandbox.kce.ksyun.com",
		WebhookServer:          crwebhook.NewServer(crwebhook.Options{Port: 9443}),
	})
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	openapiClient := openapi.NewClient(openapiBaseURL)
	openapiClient.Service = openapiService
	openapiClient.Version = openapiVersion
	openapiClient.AuthMode = openapiAuthMode
	credentialManager := credentials.NewManager(mgr.GetClient(), defaultOpenAPISecretName)
	operationRecorder := operation.NewRecorder(mgr.GetClient())
	operatorNamespace := envOrDefault("OPERATOR_NAMESPACE", envOrDefault("POD_NAMESPACE", "sandbox-operator-system"))
	operatorUsername := envOrDefault("OPERATOR_USERNAME", fmt.Sprintf("system:serviceaccount:%s:sandbox-operator", operatorNamespace))

	if err := (&controller.SandboxTemplateReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Credentials: credentialManager, OpenAPI: openapiClient, Operations: operationRecorder}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create SandboxTemplate controller")
		os.Exit(1)
	}
	if err := (&controller.SandboxReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Credentials: credentialManager, OpenAPI: openapiClient, Operations: operationRecorder}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create Sandbox controller")
		os.Exit(1)
	}
	if err := (&controller.SandboxClaimReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Operations: operationRecorder}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create SandboxClaim controller")
		os.Exit(1)
	}

	templateHandler := sandboxwebhook.NewHandler(mgr.GetClient(), mgr.GetScheme(), credentialManager, operationRecorder, openapiClient, "SandboxTemplate")
	templateHandler.OperatorUsername = operatorUsername
	sandboxHandler := sandboxwebhook.NewHandler(mgr.GetClient(), mgr.GetScheme(), credentialManager, operationRecorder, openapiClient, "Sandbox")
	sandboxHandler.OperatorUsername = operatorUsername
	claimHandler := sandboxwebhook.NewHandler(mgr.GetClient(), mgr.GetScheme(), credentialManager, operationRecorder, openapiClient, "SandboxClaim")
	claimHandler.OperatorUsername = operatorUsername

	mgr.GetWebhookServer().Register("/validate-sandbox-kce-ksyun-com-v1alpha1-sandboxtemplate", &admission.Webhook{Handler: templateHandler})
	mgr.GetWebhookServer().Register("/validate-sandbox-kce-ksyun-com-v1alpha1-sandbox", &admission.Webhook{Handler: sandboxHandler})
	mgr.GetWebhookServer().Register("/validate-sandbox-kce-ksyun-com-v1alpha1-sandboxclaim", &admission.Webhook{Handler: claimHandler})

	poller := &controller.Poller{
		Client:                  mgr.GetClient(),
		Credentials:             credentialManager,
		OpenAPI:                 openapiClient,
		Operations:              operationRecorder,
		Interval:                pollInterval,
		PageSize:                pollPageSize,
		MaxConcurrentNamespaces: maxConcurrentNamespaces,
		AdoptExternal:           true,
	}
	if err := mgr.Add(poller); err != nil {
		ctrl.Log.Error(err, "unable to add poller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctrl.Log.Info("starting sandbox operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func envBoolOrDefault(key string, defaultValue bool) bool {
	switch os.Getenv(key) {
	case "true", "1", "yes", "y":
		return true
	case "false", "0", "no", "n":
		return false
	default:
		return defaultValue
	}
}

func envDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func envIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
