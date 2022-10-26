package guardrails

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	arov1alpha1 "github.com/Azure/ARO-RP/pkg/operator/apis/aro.openshift.io/v1alpha1"
	aroclient "github.com/Azure/ARO-RP/pkg/operator/clientset/versioned"
	"github.com/Azure/ARO-RP/pkg/operator/controllers/guardrails/config"
	"github.com/Azure/ARO-RP/pkg/util/deployer"
	"github.com/Azure/ARO-RP/pkg/util/dynamichelper"
	"github.com/Azure/ARO-RP/pkg/util/version"
	"github.com/sirupsen/logrus"
)

const (
	ControllerName               = "GuardRails"
	controllerEnabled            = "aro.guardrails.enabled"        // boolean, false by default
	controllerNamespace          = "aro.guardrails.namespace"      // string
	controllerManaged            = "aro.guardrails.deploy.managed" // trinary, do-nothing by default
	controllerPullSpec           = "aro.guardrails.deploy.pullspec"
	controllerManagerRequestsCPU = "aro.guardrails.deploy.manager.requests.cpu"
	controllerManagerRequestsMem = "aro.guardrails.deploy.manager.requests.mem"
	controllerManagerLimitCPU    = "aro.guardrails.deploy.manager.limit.cpu"
	controllerManagerLimitMem    = "aro.guardrails.deploy.manager.limit.mem"
	controllerAuditRequestsCPU   = "aro.guardrails.deploy.audit.requests.cpu"
	controllerAuditRequestsMem   = "aro.guardrails.deploy.audit.requests.mem"
	controllerAuditLimitCPU      = "aro.guardrails.deploy.audit.limit.cpu"
	controllerAuditLimitMem      = "aro.guardrails.deploy.audit.limit.mem"
	// controllerWebhookManaged        = "aro.guardrails.webhook.managed"        // trinary, do-nothing by default
	// controllerWebhookTimeout        = "aro.guardrails.webhook.timeoutSeconds" // int, 3 by default (as per upstream)
	// controllerReconciliationMinutes = "aro.guardrails.reconciliationMinutes"  // int, 60 by default.

	defaultNamespace = "openshift-azure-guardrails"

	defaultManagerRequestsCPU = "100m"
	defaultManagerLimitCPU    = "1000m"
	defaultManagerRequestsMem = "256Mi"
	defaultManagerLimitMem    = "512Mi"

	defaultAuditRequestsCPU = "100m"
	defaultAuditLimitCPU    = "1000m"
	defaultAuditRequestsMem = "256Mi"
	defaultAuditLimitMem    = "512Mi"
)

//go:embed staticresources
var staticFiles embed.FS

//go:embed gktemplates
var gkPolicyTemplates embed.FS

//go:embed gkcontraints
var gkPolicyConraints embed.FS

var pullSecretName = types.NamespacedName{Name: "pull-secret", Namespace: "openshift-config"}

type Reconciler struct {
	arocli             aroclient.Interface
	kubernetescli      kubernetes.Interface
	deployer           deployer.Deployer
	gkPolicyTemplate   deployer.Deployer
	gkPolicyConstraint deployer.Deployer

	readinessPollTime time.Duration
	readinessTimeout  time.Duration
}

func NewReconciler(arocli aroclient.Interface, kubernetescli kubernetes.Interface, dh dynamichelper.Interface) *Reconciler {
	return &Reconciler{
		arocli:             arocli,
		kubernetescli:      kubernetescli,
		deployer:           deployer.NewDeployer(kubernetescli, dh, staticFiles, "staticresources"),
		gkPolicyTemplate:   deployer.NewDeployer(kubernetescli, dh, gkPolicyTemplates, "gktemplates"),
		gkPolicyConstraint: deployer.NewDeployer(kubernetescli, dh, gkPolicyConraints, "gkcontraints"),

		readinessPollTime: 10 * time.Second,
		readinessTimeout:  5 * time.Minute,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	instance, err := r.arocli.AroV1alpha1().Clusters().Get(ctx, arov1alpha1.SingletonClusterName, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}

	if !instance.Spec.OperatorFlags.GetSimpleBoolean(controllerEnabled) {
		// controller is disabled
		return reconcile.Result{}, nil
	}

	managed := instance.Spec.OperatorFlags.GetWithDefault(controllerManaged, "")

	// If enabled and managed=true, install GuardRails
	// If enabled and managed=false, remove the GuardRails deployment
	// If enabled and managed is missing, do nothing
	if strings.EqualFold(managed, "true") {
		// apply the default pullspec if the flag is empty or missing
		pullSpec := instance.Spec.OperatorFlags.GetWithDefault(controllerPullSpec, "")
		if pullSpec == "" {
			pullSpec = version.GateKeeperImage(instance.Spec.ACRDomain)
		}

		deployConfig := &config.GuardRailsDeploymentConfig{
			Pullspec:  pullSpec,
			Namespace: instance.Spec.OperatorFlags.GetWithDefault(controllerNamespace, defaultNamespace),

			ManagerRequestsCPU: instance.Spec.OperatorFlags.GetWithDefault(controllerManagerRequestsCPU, defaultManagerRequestsCPU),
			ManagerLimitCPU:    instance.Spec.OperatorFlags.GetWithDefault(controllerManagerLimitCPU, defaultManagerLimitCPU),
			ManagerRequestsMem: instance.Spec.OperatorFlags.GetWithDefault(controllerManagerRequestsMem, defaultManagerRequestsMem),
			ManagerLimitMem:    instance.Spec.OperatorFlags.GetWithDefault(controllerManagerLimitMem, defaultManagerLimitMem),

			AuditRequestsCPU: instance.Spec.OperatorFlags.GetWithDefault(controllerAuditRequestsCPU, defaultAuditRequestsCPU),
			AuditLimitCPU:    instance.Spec.OperatorFlags.GetWithDefault(controllerAuditLimitCPU, defaultAuditLimitCPU),
			AuditRequestsMem: instance.Spec.OperatorFlags.GetWithDefault(controllerAuditRequestsMem, defaultAuditRequestsMem),
			AuditLimitMem:    instance.Spec.OperatorFlags.GetWithDefault(controllerAuditLimitMem, defaultAuditLimitMem),
		}

		// Deploy the GateKeeper manifests and config
		err = r.deployer.CreateOrUpdate(ctx, instance, deployConfig)
		if err != nil {
			logrus.Printf("\x1b[%dm guardrails:: reconcile error updating %s\x1b[0m", 31, err.Error())
			return reconcile.Result{}, err
		}

		// Check that GuardRails has become ready, wait up to readinessTimeout (default 5min)
		timeoutCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
		defer cancel()

		err := wait.PollImmediateUntil(r.readinessPollTime, func() (bool, error) {
			if ready, err := r.deployer.IsReady(ctx, deployConfig.Namespace, "gatekeeper-audit"); !ready || err != nil {
				return ready, err
			}
			return r.deployer.IsReady(ctx, deployConfig.Namespace, "gatekeeper-controller-manager")
		}, timeoutCtx.Done())
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("GateKeeper deployment timed out on Ready: %w", err)
		}

		policyConfig := &config.GuardRailsPolicyConfig{}
		if r.gkPolicyTemplate != nil && r.gkPolicyConstraint != nil {
			logrus.Printf("\x1b[%dm guardrails:: creating gkPolicyTemplate for %v\x1b[0m", 31, r.gkPolicyTemplate)
			// Deploy the GateKeeper policies templates
			err = r.gkPolicyTemplate.CreateOrUpdate(ctx, instance, policyConfig)
			if err != nil {
				logrus.Printf("\x1b[%dm guardrails:: reconcile error setup template %s\x1b[0m", 31, err.Error())
				return reconcile.Result{}, err
			}
			logrus.Printf("\x1b[%dm guardrails:: creating gkPolicyConstraint for %v\x1b[0m", 31, r.gkPolicyConstraint)
			// Deploy the GateKeeper policies contraints
			err = r.gkPolicyConstraint.CreateOrUpdate(ctx, instance, policyConfig)
			if err != nil {
				logrus.Printf("\x1b[%dm guardrails:: reconcile error setup constraints %s\x1b[0m", 31, err.Error())
				return reconcile.Result{}, err
			}
		}
		// TODO: need to find a way to check if gatekeeper policies have been deployed successfully
		// Check that GuardRails policies has become ready, wait up to readinessTimeout (default 5min)
		// timeoutPolicyCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
		// defer cancel()

		// err = wait.PollImmediateUntil(r.readinessPollTime, func() (bool, error) {
		// 	if ready, err := r.gkPolicyTemplate.IsReady(ctx, "", "arodenylabels"); !ready || err != nil { //  ConstraintTemplate
		// 		return ready, err
		// 	}
		// 	return r.gkPolicyConstraint.IsReady(ctx, "", "aro-machines-deny") // arodenylabels
		// }, timeoutPolicyCtx.Done())
		// if err != nil {
		// 	return reconcile.Result{}, fmt.Errorf("GateKeeper policy timed out on Ready: %w", err)
		// }

		// todo: start a timer to periodically re-enforce gatekeeper policies, in case they are deleted by users?

	} else if strings.EqualFold(managed, "false") {
		if r.gkPolicyTemplate != nil && r.gkPolicyConstraint != nil {
			err := r.gkPolicyConstraint.Remove(ctx, config.GuardRailsPolicyConfig{})
			if err != nil {
				return reconcile.Result{}, err
			}
			err = r.gkPolicyTemplate.Remove(ctx, config.GuardRailsPolicyConfig{})
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		err = r.deployer.Remove(ctx, config.GuardRailsDeploymentConfig{Namespace: instance.Spec.OperatorFlags.GetWithDefault(controllerNamespace, defaultNamespace)})
		if err != nil {
			logrus.Printf("\x1b[%dm guardrails:: reconcile error removing deployment %s\x1b[0m", 31, err.Error())
			return reconcile.Result{}, err
		}

		// todo: disable the gatekeeper policies
	}

	return reconcile.Result{}, nil
}

// SetupWithManager setup our manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {

	pullSecretPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return (o.GetName() == pullSecretName.Name && o.GetNamespace() == pullSecretName.Namespace)
	})

	aroClusterPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == arov1alpha1.SingletonClusterName
	})

	grBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&arov1alpha1.Cluster{}, builder.WithPredicates(aroClusterPredicate)).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(pullSecretPredicate),
		)

	resources, err := r.deployer.Template(&config.GuardRailsDeploymentConfig{}, staticFiles)
	if err != nil {
		return err
	}

	for _, i := range resources {
		o, ok := i.(client.Object)
		if ok {
			grBuilder.Owns(o)
		}
	}

	// we won't listen for changes on policies, since we only want to reconcile on a timer anyway
	if err := grBuilder.
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}, predicate.LabelChangedPredicate{})).
		Named(ControllerName).
		Complete(r); err != nil {
		logrus.Printf("\x1b[%dm guardrails::SetupWithManager deployment failed %v 0\x1b[0m", 31, err)
		return err
	}
	return nil
}
