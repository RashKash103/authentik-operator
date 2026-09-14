/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command manager runs the authentik operator.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
	"rka.sh/authentik-operator/internal/controller"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(authentikv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var secureMetrics bool
	var enableHTTP2 bool
	var watchNamespaces string

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0",
		"The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager, ensuring only one active manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"Serve the metrics endpoint over HTTPS with authn/authz.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"Enable HTTP/2 for the metrics and webhook servers.")
	flag.StringVar(&watchNamespaces, "watch-namespaces", "",
		"Comma-separated list of namespaces to watch. Empty means all namespaces.")

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// HTTP/2 is disabled by default: it has a history of DoS-shaped CVEs
	// (Rapid Reset et al) and the operator's own endpoints gain nothing from it.
	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		})
	}

	metricsOpts := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsOpts.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	cacheOpts := cache.Options{}
	if watchNamespaces != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{}
		for _, ns := range strings.Split(watchNamespaces, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				cacheOpts.DefaultNamespaces[ns] = cache.Config{}
			}
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOpts,
		Cache:                  cacheOpts,
		WebhookServer:          webhook.NewServer(webhook.Options{TLSOpts: tlsOpts}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "authentik-operator.rka.sh",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Controllers are registered here as they land; see setupControllers.
	if err := setupControllers(mgr); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "version", version)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupControllers wires every reconciler into the manager.
//
// All controllers share one ConnectionResolver, and therefore one reference
// cache, so a flow slug resolved for one resource does not have to be looked up
// again for the next. Entries stay partitioned per authentik instance.
func setupControllers(mgr ctrl.Manager) error {
	resolver := &controller.ConnectionResolver{
		Client:          mgr.GetClient(),
		Cache:           authentik.NewRefCache(authentik.DefaultCacheTTL),
		UserAgentSuffix: version,
	}

	if err := (&controller.AuthentikConnectionReconciler{
		Client:   mgr.GetClient(),
		Resolver: resolver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create AuthentikConnection controller: %w", err)
	}

	if err := (&controller.ClusterAuthentikConnectionReconciler{
		Client:   mgr.GetClient(),
		Resolver: resolver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create ClusterAuthentikConnection controller: %w", err)
	}

	if err := (&controller.OAuth2ProviderReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("oauth2provider-controller"),
		Resolver: resolver,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create OAuth2Provider controller: %w", err)
	}

	return nil
}
