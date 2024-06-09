/*
Copyright 2024.

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

package controller

import (
	"context"
	"errors"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strconv"
	"time"

	upcloudSdk "github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/buraksekili/upcloud-operator/api/v1alpha1"
	"github.com/buraksekili/upcloud-operator/internal/upcloud"
	"github.com/go-logr/logr"
	"github.com/mitchellh/hashstructure/v2"
	k8sErr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	pollingDuration = 1 * time.Minute
	deleteDuration  = 30 * time.Second

	finalizerKeyVM = "finalizers.upcloud/vm"
)

var (
	requeueAfter1Min   = ctrl.Result{RequeueAfter: pollingDuration}
	requeueAfter30Secs = ctrl.Result{RequeueAfter: deleteDuration}
)

// VMReconciler reconciles a VM object
type VMReconciler struct {
	UpClient upcloud.Client
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=upcloud.buraksekili.github.io,resources=vms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=upcloud.buraksekili.github.io,resources=vms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=upcloud.buraksekili.github.io,resources=vms/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VM object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *VMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("vm", req.NamespacedName)
	r.UpClient.Logger = l

	desired := v1alpha1.VM{}
	if err := r.Get(ctx, req.NamespacedName, &desired); err != nil {
		if !k8sErr.IsNotFound(err) {
			l.Error(err, "failed to fetch VM")
		}

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("reconciling VM instance")

	if !desired.ObjectMeta.DeletionTimestamp.IsZero() {
		l.Info("VM instance being deleted")
		return r.reconcileDelete(ctx, l, &desired)
	}

	if finalizerAdded := controllerutil.AddFinalizer(&desired, finalizerKeyVM); finalizerAdded {
		return ctrl.Result{}, r.Update(ctx, &desired)
	}

	if desired.Status.UUID != "" {
		return r.reconcileExistingVM(ctx, l, &desired)
	}

	return r.reconcileNewVM(ctx, &desired) // it might exist on upcloud, check it
}

func ignoreUpdateEvents() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil {
				return false
			}
			if e.ObjectNew == nil {
				return false
			}

			if !reflect.DeepEqual(e.ObjectNew.GetFinalizers(), e.ObjectOld.GetFinalizers()) ||
				(e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()) {
				return true
			}

			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *VMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(ignoreUpdateEvents()).
		For(&v1alpha1.VM{}).
		Complete(r)
}

// reconcileDelete handles delete operations. It first stops the VM and schedules a reconciliation for 30 seconds later.
// Once the VM has been stopped, it deletes the VM and removes finalizers.
func (r *VMReconciler) reconcileDelete(ctx context.Context, l logr.Logger, vm *v1alpha1.VM) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(vm, finalizerKeyVM) {
		_, err := r.UpClient.StopVM(ctx, vm)
		var problem *upcloudSdk.Problem
		if err != nil {
			if errors.As(err, &problem) {
				if problem.ErrorCode() == upcloudSdk.ErrCodeNotFound || problem.ErrorCode() == upcloudSdk.ErrCodeServerNotFound {
					if finalizerDeleted := controllerutil.RemoveFinalizer(vm, finalizerKeyVM); finalizerDeleted {
						return ctrl.Result{}, r.Update(ctx, vm)
					}
				}
				if problem.ErrorCode() != upcloudSdk.ErrCodeServerStateIllegal {
					l.Error(err, "failed to stop VM", "UUID", vm.Status.UUID)
					return ctrl.Result{}, err
				}
			} else {
				l.Error(err, "failed to stop VM", "UUID", vm.Status.UUID)
				return ctrl.Result{}, err
			}
		}

		l.Info("UpCloud will delete the server eventually")
		serverDetails, err := r.UpClient.GetVM(ctx, vm.Status.UUID)
		if err != nil {
			l.Error(err, "failed to get VM", "UUID", vm.Status.UUID)
			return ctrl.Result{}, err
		}

		if serverDetails.State != upcloudSdk.ServerStateStopped {
			l.Info("The server is not stopped yet, will requeue after 30 secs")
			serverDetails.State = upcloudSdk.ServerStateMaintenance
			return requeueAfter30Secs, r.updateStatus(ctx, vm, serverDetails)
		}

		l.Info("The server state is stopped, try deleting the server", "UUID", vm.Status.UUID)

		err = r.UpClient.DeleteVM(ctx, vm)
		if err != nil {
			l.Error(err, "failed to delete VM", "UUID", vm.Status.UUID)
			return ctrl.Result{}, err
		}

		l.Info("The server has been deleted successfully from UpCloud")

		if finalizerDeleted := controllerutil.RemoveFinalizer(vm, finalizerKeyVM); finalizerDeleted {
			return ctrl.Result{}, r.Update(ctx, vm)
		}
	}

	return ctrl.Result{}, nil
}

// updateStatus updates given VM CR's status based on given VM details if needed.
func (r *VMReconciler) updateStatus(ctx context.Context, vm *v1alpha1.VM, sd *upcloudSdk.ServerDetails) error {
	if sd == nil {
		return errors.New("failed to update status, nil ServerDetails")
	}

	hStr := calculateHash(sd)
	if vm.Status.UUID != sd.UUID || vm.Status.State != sd.State ||
		vm.Status.Title != sd.Title || vm.Status.UpCloudVmHash != hStr {
		vm.Status.UUID = sd.UUID
		vm.Status.State = sd.State
		vm.Status.Title = sd.Title
		vm.Status.UpCloudVmHash = hStr

		if err := r.Status().Update(ctx, vm); err != nil {
			return err
		}

		return nil
	}

	return nil
}

func (r *VMReconciler) reconcileExistingVM(ctx context.Context, l logr.Logger, vm *v1alpha1.VM) (ctrl.Result, error) {
	l.Info("reconciling existing VM CR")
	if existingVMDetails, existsOnUpCloud := r.UpClient.Exists(ctx, vm.Status.UUID); existsOnUpCloud {
		l.Info("VM exists on UpCloud")
		// if the vm exists on UpCloud, update it based on k8s desired state.
		if existingVMDetails.State == upcloudSdk.ServerStateStopped {
			go func() {
				l.Info("the server has stopped, try starting it again", "UUID", vm.Status.UUID)
				if _, err := r.UpClient.StartVM(ctx, vm); err != nil {
					l.Error(err, "failed to start VM")
				}
			}()
			existingVMDetails.State = upcloudSdk.ServerStateMaintenance

			return requeueAfter1Min, r.updateStatus(ctx, vm, existingVMDetails)
		}

		newServerDetails, err := r.UpClient.CreateOrUpdateVM(ctx, vm)
		if err != nil {
			l.Error(err, "failed to create or update existing VM")
			return ctrl.Result{}, err
		}

		err = r.updateStatus(ctx, vm, newServerDetails)
		if err != nil {
			l.Error(err, "failed to update status")
			return ctrl.Result{}, err
		}

		l.Info("VM Status is updated")
		return requeueAfter1Min, nil
	}

	l.Info("VM missing in UpCloud, try recreating the VM")

	// there might be a drift between UpCloud and k8s state which means k8s assumes the VM exists by looking at
	// .status.UUID field. However, UpCloud API returns non-existing response. So, VM needs to be recreated if
	// the server does not exist on UpCloud.
	serverDetails, err := r.UpClient.CreateOrUpdateVM(ctx, vm)
	if err != nil {
		l.Error(err, "failed to recreate VM")
		return ctrl.Result{}, err
	}

	return requeueAfter1Min, r.updateStatus(ctx, vm, serverDetails)
}

// reconcileNewVM handles reconciliation of VM create requests.
func (r *VMReconciler) reconcileNewVM(ctx context.Context, vm *v1alpha1.VM) (ctrl.Result, error) {
	sd, err := r.UpClient.CreateOrUpdateVM(ctx, vm)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.updateStatus(ctx, vm, sd); err != nil {
		return ctrl.Result{}, err
	}

	return requeueAfter1Min, err
}

var hashOptions = hashstructure.HashOptions{ZeroNil: true}

// calculateHash calculates given interface's hash value.
func calculateHash(i interface{}) (h string) {
	h1, err1 := hashstructure.Hash(i, hashstructure.FormatV2, &hashOptions)
	if err1 == nil {
		h = strconv.FormatUint(h1, 10)
	}

	return
}
