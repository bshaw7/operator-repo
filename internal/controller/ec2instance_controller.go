package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	computev1 "github.com/bshaw7/operator-repo/api/v1"
)

// Ec2InstanceReconciler is a struct that implements the logic for reconciling Ec2Instance custom resources.
// It embeds the Kubernetes client.Client interface, which provides methods for interacting with the Kubernetes API server,
// and holds a pointer to a runtime.Scheme, which is used for type conversions between Go structs and Kubernetes objects.
// This struct is used to reconcile the Ec2Instance custom resource.

type Ec2InstanceReconciler struct {
	client.Client                 // Used to perform CRUD operations on Kubernetes resources.
	Scheme        *runtime.Scheme // Used to map Go types to Kubernetes GroupVersionKinds and vice versa.
}

/* Following are "Markers": These comments are special markers that the controller-gen tool (part of the Kubebuilder framework) understands.
Used for Code Generation: When you run make manifests in your project, controller-gen reads these markers and automatically generates the ClusterRole YAML manifest file.
This file defines all the permissions your controller needs to interact with Kubernetes API objects.
*/
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// After updating the status of the resource (e.g., with r.Status().Update), the Kubernetes API server
// will emit an update event for the resource. This event will be picked up by the controller-runtime
// and will cause the Reconcile function to be called again for the same resource. This is why, after
// updating the status, the reconciler is called again: it is a result of the Kubernetes watch mechanism
// and ensures that the controller can observe and react to any changes, including those it made itself.
// This pattern is common in Kubernetes controllers to ensure eventual consistency and to handle
// situations where the status update may not have been fully applied or observed yet.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *Ec2InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// TODO(user): your logic here
	l.Info("=== RECONCILE LOOP STARTED ===", "namespace", req.Namespace, "name", req.Name)

	// Creating instance of the Ec2Instance struct to hold the data retrieved from the Kubernetes API
	ec2Instance := &computev1.Ec2Instance{}
	// Retrieving the Ec2Instance resource from the Kubernetes API server using the provided request's NamespacedName.
	if err := r.Get(ctx, req.NamespacedName, ec2Instance); err != nil {
		if errors.IsNotFound(err) {
			l.Info("Instance Deleted. No need to reconcile")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	//check if deletionTimestamp is not zero
	if !ec2Instance.DeletionTimestamp.IsZero() {
		l.Info("Has deletionTimestamp, Instance is being deleted")
		_, err := deleteEc2Instance(ctx, ec2Instance)
		if err != nil {
			l.Error(err, "Failed to delete EC2 instance")
			return ctrl.Result{Requeue: true}, err
		}

		// Remove the finalizer
		controllerutil.RemoveFinalizer(ec2Instance, "ec2instance.compute.cloud.com")
		if err := r.Update(ctx, ec2Instance); err != nil {
			l.Error(err, "Failed to remove finalizer")
			// Kubernetes will retry with backoff
			return ctrl.Result{Requeue: true}, err
		}
		return ctrl.Result{}, nil
	}

	// Check if we already have an instance ID in status

	// OLD code which only check instance id in k8s resource not on aws
	// if ec2Instance.Status.InstanceID != "" {
	// 	l.Info("Requested object already exists in Kubernetes. Not creating a new instance.", "instanceID", ec2Instance.Status.InstanceID)

	// 	return ctrl.Result{}, nil
	// }

	// New logic to check in k8s and aws as well if instance already exist

	if ec2Instance.Status.InstanceID != "" {
		// 1. USE THE UNUSED FUNCTION: Check AWS Reality
		exists, awsInstance, err := checkEC2InstanceExists(ctx, ec2Instance.Status.InstanceID, ec2Instance)
		if err != nil {
			l.Error(err, "Failed to check if instance exists in AWS")
			return ctrl.Result{}, err
		}

		// 2. SELF-HEALING: If AWS says "Not Found" or "Terminated"
		if !exists || string(awsInstance.State.Name) == "terminated" {
			l.Info("Instance found in Status but missing/terminated in AWS. Triggering recreation.", "ID", ec2Instance.Status.InstanceID)

			// Reset the Status ID to empty.
			// In the NEXT loop, the operator will see empty ID and create a new one.
			ec2Instance.Status.InstanceID = ""
			ec2Instance.Status.State = "Terminated"
			if err := r.Status().Update(ctx, ec2Instance); err != nil {
				l.Error(err, "Failed to reset status for recreation")
				return ctrl.Result{}, err
			}
			// Requeue immediately to start fresh
			return ctrl.Result{Requeue: true}, nil
		}

		// 3. SYNC STATE: If it exists, update the status to match AWS (e.g. "pending" -> "running")
		if ec2Instance.Status.State != string(awsInstance.State.Name) {
			l.Info("Updating Instance State", "Old", ec2Instance.Status.State, "New", awsInstance.State.Name)
			ec2Instance.Status.State = string(awsInstance.State.Name)
			if err := r.Status().Update(ctx, ec2Instance); err != nil {
				return ctrl.Result{}, err
			}
		}

		// It exists and is healthy. Stop.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	l.Info("Creating new instance")

	l.Info("=== ABOUT TO ADD FINALIZER ===")
	ec2Instance.Finalizers = append(ec2Instance.Finalizers, "ec2instance.compute.cloud.com")
	if err := r.Update(ctx, ec2Instance); err != nil {
		l.Error(err, "Failed to add finalizer")
		return ctrl.Result{
			Requeue: true,
		}, err
	}
	l.Info("=== FINALIZER ADDED - This update will trigger a NEW reconcile loop, but current reconcile continues ===")

	// Create a new instance
	l.Info("=== CONTINUING WITH EC2 INSTANCE CREATION IN CURRENT RECONCILE ===")

	createdInstanceInfo, err := createEc2Instance(ec2Instance)
	if err != nil {
		l.Error(err, "Failed to create EC2 instance")
		return ctrl.Result{}, err
	}

	l.Info("=== ABOUT TO UPDATE STATUS - This will trigger reconcile loop again ===",
		"instanceID", createdInstanceInfo.InstanceID,
		"state", createdInstanceInfo.State)

	ec2Instance.Status.InstanceID = createdInstanceInfo.InstanceID
	ec2Instance.Status.State = createdInstanceInfo.State
	ec2Instance.Status.PublicIP = createdInstanceInfo.PublicIP
	ec2Instance.Status.PrivateIP = createdInstanceInfo.PrivateIP
	ec2Instance.Status.PublicDNS = createdInstanceInfo.PublicDNS
	ec2Instance.Status.PrivateDNS = createdInstanceInfo.PrivateDNS

	err = r.Status().Update(ctx, ec2Instance)
	if err != nil {
		l.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}
	l.Info("=== STATUS UPDATED - Reconcile loop will be triggered again ===")

	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
// SetupWithManager registers the Ec2InstanceReconciler with the controller manager.
// It configures the controller to watch for changes to Ec2Instance resources.
// The controller will be named "ec2instance" for logging and metrics purposes.
// The Complete(r) call finalizes the setup, associating the reconciler logic with this controller.
func (r *Ec2InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&computev1.Ec2Instance{}).
		Named("ec2instance").
		Complete(r)
}
