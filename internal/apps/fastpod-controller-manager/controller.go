/*
Copyright 2024 FaST-GShare Authors, KontonGu (Jianfeng Gu), et. al.
@Techinical University of Munich, CAPS Cloud Team

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

package fastpodcontrollermanager

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/KontonGu/FaST-GShare/internal/db/ent"
	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
	clientset "github.com/KontonGu/FaST-GShare/pkg/client/clientset/versioned"
	fastpodscheme "github.com/KontonGu/FaST-GShare/pkg/client/clientset/versioned/scheme"
	informers "github.com/KontonGu/FaST-GShare/pkg/client/informers/externalversions/fastgshare.caps.in.tum/v1"
	listers "github.com/KontonGu/FaST-GShare/pkg/client/listers/fastgshare.caps.in.tum/v1"
	"github.com/KontonGu/FaST-GShare/pkg/types"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/go-logr/logr"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
	k8scontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
)

const controllerAgentName = "fastpod-controller"

var (
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc // can still create the key even the object is deleted;
)

type Controller struct {
	kubeClient    kubernetes.Interface
	fastpodClient clientset.Interface

	podsLister  corelisters.PodLister
	podsSynced  cache.InformerSynced
	podInformer coreinformers.PodInformer

	fastpodsLister listers.FaSTPodLister
	fastpodsSynced cache.InformerSynced

	nodesLister corelisters.NodeLister
	nodesSynced cache.InformerSynced

	expectations *k8scontroller.UIDTrackingControllerExpectations

	pendingList    *list.List
	pendingListMux *sync.Mutex

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// containerdClient *containerd.Client

	db  *ent.Client
	log *logr.Logger
}

// NewController returns a new FaSTPod controller
func NewController(
	kubeclient kubernetes.Interface,
	fastpodclient clientset.Interface,
	nodeinformer coreinformers.NodeInformer,
	podinformer coreinformers.PodInformer,
	fastpodinformer informers.FaSTPodInformer,
	dbClient *ent.Client,
) *Controller {

	// Create event broadcaster
	// Add fastpod-controller types to the default Kubernetes Scheme so Events can be
	// logged for fastpod-controller types.
	utilruntime.Must(fastpodscheme.AddToScheme(kubescheme.Scheme))
	ctrLogger := klog.NewKlogr().WithName("fastpod-controller")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(kubescheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeClient:    kubeclient,
		fastpodClient: fastpodclient,

		podsLister:  podinformer.Lister(),
		podsSynced:  podinformer.Informer().HasSynced,
		podInformer: podinformer,

		fastpodsLister: fastpodinformer.Lister(),
		fastpodsSynced: fastpodinformer.Informer().HasSynced,

		nodesLister: nodeinformer.Lister(),
		nodesSynced: nodeinformer.Informer().HasSynced,

		expectations: k8scontroller.NewUIDTrackingControllerExpectations(k8scontroller.NewControllerExpectations()),

		pendingList:    list.New(),
		pendingListMux: &sync.Mutex{},

		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "FaSTPods"),
		recorder:  recorder,

		db:  dbClient,
		log: &ctrLogger,
	}

	ctrLogger.Info("setting up event handlers")
	// Set up an event handler for when FaSTPod resources change
	fastpodinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFaSTPod,
		UpdateFunc: func(old, new interface{}) {
			newFstp := new.(*fastpodv1.FaSTPod)
			oldFstp := old.(*fastpodv1.FaSTPod)
			controller.log.Info(fmt.Sprintf("updatefunc: current fastpod %s with replica %d ", newFstp.Name, *newFstp.Spec.Replicas))
			controller.log.Info(fmt.Sprintf("updatefunc: queue length %d", controller.workqueue.Len()))
			if newFstp.ResourceVersion != oldFstp.ResourceVersion {
				controller.log.Info("fastpod has different resourceversion, update the fastpod.")
				controller.enqueueFaSTPod(new)
				return
			}

			controller.enqueueFaSTPod(new)
		},
		DeleteFunc: controller.handleDeletedFaSTPod,
	})

	// Set up an event handler for when Pod resources change. This
	// handler will lookup the owner of the given Pod, and if it is
	// owned by a FaSTPod resource will enqueue that FaSTPod resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Pod resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	podinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// AddFunc: controller.handleObject,
		// UpdateFunc: func(old, new interface{}) {
		// 	newDepl := new.(*corev1.Pod)
		// 	oldDepl := old.(*corev1.Pod)
		// 	if newDepl.ResourceVersion == oldDepl.ResourceVersion {
		// 		// controller.handleObject(new)
		// 		return
		// 	}
		// 	controller.handleObject(new)
		// },
		//TODO release pod when scale down?
		DeleteFunc: controller.handleObject,
	})

	nodeinformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: controller.resourceChanged,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until context
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.

func (ctr *Controller) Run(stopCh <-chan struct{}, workers int, configNetAddr string) error {
	defer utilruntime.HandleCrash()
	defer ctr.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	ctr.log.Info("starting fastpod controller")

	// Wait for the caches to be synced before starting workers
	ctr.log.Info("waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, ctr.podsSynced, ctr.fastpodsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	// if err := ctr.gpuNodeInit(); err != nil {
	// 	return fmt.Errorf("Error failed to init the gpu nodes: %s", err)
	// }

	pendingInsuranceTicker := time.NewTicker(5 * time.Second)
	pendingInsuranceDone := make(chan bool)
	go ctr.pendingInsurance(pendingInsuranceTicker, &pendingInsuranceDone)

	go ctr.startConfigManager(stopCh, ctr.kubeClient, configNetAddr)
	ctr.log.Info(fmt.Sprintf("starting workers, numuber of workers = %d.", workers))
	// Launch two workers to process FaSTPod resources
	for i := 0; i < workers; i++ {
		go wait.Until(ctr.runWorker, time.Second, stopCh)
	}
	ctr.log.Info("workers started")
	<-stopCh
	ctr.log.Info("shutting down workers")
	pendingInsuranceTicker.Stop()
	pendingInsuranceDone <- true
	return nil

}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (ctr *Controller) runWorker() {
	for ctr.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (ctr *Controller) processNextWorkItem() bool {
	objRef, shutdown := ctr.workqueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer ctr.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			ctr.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("the object in the workqueue is not valid. obj=%#v", obj))
			return nil
		}
		if err := ctr.syncHandler(key); err != nil {
			if err.Error() == "Waiting4Dummy" {
				ctr.workqueue.Add(key)
				return fmt.Errorf("testing: need to wait for dummy pod '#{key}', requeueing")
			}

			ctr.workqueue.AddRateLimited(key)
			return fmt.Errorf("error while syncing the object = %s: %s, the object is re-queued", key, err.Error())
		}

		ctr.workqueue.Forget(obj)
		ctr.log.Info(fmt.Sprintf("successfully sync the object = %s.", key))
		return nil
	}(objRef)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the FaSTPod resource
// with the current status of the resource.
func (ctr *Controller) syncHandler(key string) error {
	startTime := time.Now()
	ctr.log.Info(fmt.Sprintf("starting to sync fastpod %q (%v)", key, time.Since(startTime)))
	defer func() {
		ctr.log.Info(fmt.Sprintf("finished syncing fastpod %q (%v)", key, time.Since(startTime)))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error invalid resource key = %s", key))
		return nil
	}
	// //KONTON_TEST
	// klog.Infof("Process in syncHandler, key=%s", key)
	// return nil
	// //KONTON_TEST END

	fastpod, err := ctr.fastpodsLister.FaSTPods(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("error fastpod '%s' is no longer existed when syncing it", key))
			return nil
		}
		return err
	}

	if fastpod.Spec.Replicas == nil {
		ctr.log.Info(fmt.Sprintf("waiting fastpod %v/%v replicas to be updated ...", namespace, name))
		return nil
	}

	fastpodCopy := fastpod.DeepCopy()

	if fastpodCopy.Spec.Selector == nil {
		fastpodCopy.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app":        fastpodCopy.Name,
				"controller": fastpodCopy.Name,
			},
		}
	}

	if fastpodCopy.Status.BoundDeviceIDs == nil {
		boundIds := make(map[string]string)
		fastpodCopy.Status.BoundDeviceIDs = &boundIds
	}

	if fastpodCopy.Status.Pod2Node == nil {
		pod2Node := make(map[string]string)
		fastpodCopy.Status.Pod2Node = &pod2Node
	}

	if fastpodCopy.Status.GPUClientPort == nil {
		gpuclientPort := make(map[string]int)
		fastpodCopy.Status.GPUClientPort = &gpuclientPort
	}

	if fastpodCopy.Status.Usage == nil {
		usages := make(map[string]fastpodv1.FaSTPodUsage)
		fastpodCopy.Status.Usage = &usages
	}

	if fastpodCopy.Status.ResourceConfig == nil {
		resourceConfig := make(map[string]string)
		fastpodCopy.Status.ResourceConfig = &resourceConfig
	}

	syncFaSTPod := true
	selector, err := metav1.LabelSelectorAsSelector(fastpod.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting pod selector to selector for the fastpod %v/%v: %v", namespace, name, err))
	}
	if selector == nil {
		ctr.log.Error(nil, fmt.Sprintf("error the selector of fastpod %s/%s is till nil...", namespace, name))
		return nil
	}
	ctr.log.Info(fmt.Sprintf("konton_test: fastpod selector string: %s.", selector.String()))
	// list pods of all FaSTPod
	allPods_tmp, err := ctr.podsLister.Pods(namespace).List(selector)
	if err != nil {
		ctr.log.Error(err, fmt.Sprintf("error cannot get pods of the fastpod = %s.", key))
		return err
	}
	ctr.log.Info(fmt.Sprintf("the total number of pods of the function %s = %d.", selector.String(), len(allPods_tmp)))
	var allPods []*corev1.Pod
	// only check the
	for _, pod := range allPods_tmp {
		if pod.ObjectMeta.Labels["controller"] == fastpod.Name {
			allPods = append(allPods, pod)
		}
	}
	ctr.log.Info(fmt.Sprintf("konton_test: num of allpods of fastpod %s = %d.", fastpod.Name, len(allPods)))

	// Ignore inactive pods
	filteredPods := filterInactivePods(allPods)
	ctr.log.Info(fmt.Sprintf("the fastpod=%s/%s now has %d pods.", fastpodCopy.Namespace, fastpodCopy.Name, len(filteredPods)))

	// reconcile the quota resource configuration
	syncReStatus := false
	var manageReError error
	reqName := fastpodv1.FaSTGShareGPUQuotaRequest
	limitName := fastpodv1.FaSTGShareGPUQuotaLimit
	smName := fastpodv1.FaSTGShareGPUSMPartition
	ctr.log.Info("checking if resource configurations change......")
	if fastpodCopy.Status.ResourceConfig == nil {
		ctr.log.Info("fastpodcopy.status.resourceconfig is null, start to create it ......")
		syncReStatus = true
		(*fastpodCopy.Status.ResourceConfig)[reqName] = fastpodCopy.ObjectMeta.Annotations[reqName]
		(*fastpodCopy.Status.ResourceConfig)[limitName] = fastpodCopy.ObjectMeta.Annotations[limitName]
		(*fastpodCopy.Status.ResourceConfig)[smName] = fastpodCopy.ObjectMeta.Annotations[smName]
	} else {
		if (*fastpodCopy.Status.ResourceConfig)[reqName] != fastpodCopy.ObjectMeta.Annotations[reqName] ||
			(*fastpodCopy.Status.ResourceConfig)[limitName] != fastpodCopy.ObjectMeta.Annotations[limitName] {
			ctr.log.Info(fmt.Sprintf("gpu resource is re-configured for the fastpod %s.", fastpodCopy.ObjectMeta.Name))
			manageReError = ctr.reconcileResourceConfig(filteredPods, fastpodCopy)
			if manageReError != nil {
				ctr.log.Error(manageReError, fmt.Sprintf("error failed to update the resource annoation for the pods of the fastpod %s.", fastpodCopy.ObjectMeta.Name))

			} else {
				syncReStatus = true
				(*fastpodCopy.Status.ResourceConfig)[reqName] = fastpodCopy.ObjectMeta.Annotations[reqName]
				(*fastpodCopy.Status.ResourceConfig)[limitName] = fastpodCopy.ObjectMeta.Annotations[limitName]
				(*fastpodCopy.Status.ResourceConfig)[smName] = fastpodCopy.ObjectMeta.Annotations[smName]
				ctr.log.Info(fmt.Sprintf("the resource configurationof fastpod %s is updated.", fastpodCopy.ObjectMeta.Name))
			}
		}
	}

	// reconcile the replicas of the fastpod
	var manageReplicasErr error
	syncReplica := false
	if syncFaSTPod {
		syncReplica, manageReplicasErr = ctr.reconcileReplicas(context.TODO(), filteredPods, fastpodCopy, key)
	}
	newStatus := getFaSTPodReplicaStatus(fastpodCopy, filteredPods, manageReplicasErr)

	ctr.log.Info(fmt.Sprintf("availablereplicas: %d, readyreplicas: %d, replicas: %d", newStatus.AvailableReplicas, newStatus.ReadyReplicas, newStatus.Replicas))

	var updatedFastpod *fastpodv1.FaSTPod
	if fastpodCopy.Status.AvailableReplicas != *(fastpodCopy.Spec.Replicas) || fastpodCopy.Status.ReadyReplicas != *(fastpodCopy.Spec.Replicas) {
		fastpodCopy.Status.AvailableReplicas = newStatus.AvailableReplicas
		fastpodCopy.Status.ReadyReplicas = newStatus.ReadyReplicas
		fastpodCopy.Status.Replicas = newStatus.Replicas
		syncReplica = true
	}

	// other Status Check
	// klog.Infof("KONTON_TEST: SyncReplica Status = %v.", syncReplica)
	if syncReStatus || syncReplica {
		ctr.log.Info("sync resource status with update.")
		updatedFastpod, err = ctr.fastpodClient.FastgshareV1().FaSTPods(fastpodCopy.Namespace).Update(context.TODO(), fastpodCopy, metav1.UpdateOptions{})
		if err != nil {
			ctr.log.Error(err, "error fastpod update failed.")
			return err
		}
	}

	if manageReError != nil {
		ctr.enqueueFaSTPod(updatedFastpod)
	}

	if manageReplicasErr != nil && updatedFastpod.Status.ReadyReplicas == *(updatedFastpod.Spec.Replicas) &&
		updatedFastpod.Status.AvailableReplicas != *(updatedFastpod.Spec.Replicas) {
		ctr.log.Info(fmt.Sprintf("(enqueue fastpod from replicas check (func: synchandler)) re-enqueue the fastpod = %s.", updatedFastpod.Name))
		ctr.enqueueFaSTPod(updatedFastpod)
	}

	return manageReplicasErr
}

// reconcile the spec.Replicas and existed replcias
func (ctr *Controller) reconcileReplicas(ctx context.Context, existedPods []*corev1.Pod, fastpod *fastpodv1.FaSTPod, key string) (bool, error) {
	syncReplica := false
	diff := len(existedPods) - int(*(fastpod.Spec.Replicas))
	ctr.log.Info(fmt.Sprintf("current fastpod = %s has %d replicas with spec = %d, diff = %d.", key, len(existedPods), int(*(fastpod.Spec.Replicas)), diff))

	fstpKey, err := KeyFunc(fastpod)
	if err != nil {
		utilruntime.HandleError((fmt.Errorf("Error failed to get key of FaSTPod = %v %#v: %v.", fastpod.Kind, fastpod, err)))
		return syncReplica, nil
	}

	fastpodCopy := fastpod.DeepCopy()

	fstp2PodsMtx.Lock()
	defer fstp2PodsMtx.Unlock()

	// To create new pods if replicas is not enough
	if diff < 0 {
		// the number of pods to create
		diff *= -1
		syncReplica = true
		ctr.expectations.ExpectCreations(fstpKey, diff)
		ctr.log.Info(fmt.Sprintf("not enough replicas for the fastpod ..., spec need %d replicas, try to create %d replicas", *fastpodCopy.Spec.Replicas, diff))
		successedNum, err := slowStartbatch(diff, k8scontroller.SlowStartInitialBatchSize, func() (*corev1.Pod, error) {

			request, err := getPodRequestFromPod(fastpodCopy)
			if err != nil {
				return nil, fmt.Errorf("failed to get pod request from fastpod: %v", err)
			}

			ok, err := validatePodRequest(request)
			if err != nil || !ok {
				ctr.recorder.Event(fastpod, corev1.EventTypeWarning, ErrValueError, "Invalid pod request")
				ctr.log.Error(err, fmt.Sprintf("error invalid pod request: %s", err))
				//we dont care if the pod is invalid
				return nil, nil
			}

			if request.AllocationType == types.AllocationTypeNone {
				ctr.log.Info("allocationtype is none, skip the pod creation.")
				return nil, nil
			}

			ctr.log.Info(fmt.Sprintf("request for pod with resource %v", request))

			selectionResult, err := ctr.ScheduleRequest(request)
			if selectionResult == nil || err != nil {
				ctr.log.Info(fmt.Sprintf("error cannot find the best node for the fastpod %s. %s", key, err))
				return nil, errors.New("NoSchedNodeAvailable")
			}

			ctr.log.Info(fmt.Sprintf("the pod of fastpod = %s is scheduled [auto/specified] to the node = %s with vgpuid = %s", key, selectionResult.NodeName, selectionResult.VGPUUUID))

			// generate the pod key for the new pod of FaSTPod
			var subpodName string
			var errCode int
			fstpName := fastpodCopy.ObjectMeta.Name
			if fstp2Pods[fstpName] == nil {
				fstp2Pods[fstpName] = list.New()
			}
			newPodName := fstpName + "-" + RandStr(5)
			subpodName = newPodName
			podKey := fmt.Sprintf("%s/%s", fastpodCopy.ObjectMeta.Namespace, subpodName)

			// get the gpu device uuid and update the pod resource configuration in configurator
			// gpuDevUUID, errCode = ctr.getGPUDevUUIDAndUpdateConfig(schedNode, schedvGPUID, quotaReq, quotaLimit, smPartition, gpuMem, podKey, &gpuClientPort)
			// klog.Infof("The pod = %s of FaSTPod %s with vGPUID = %s is bound to device UUID=%s with GPUClientPort=%d.", podKey, key, schedvGPUID, gpuDevUUID, gpuClientPort)

			allocation, errCode := ctr.RequestGPUAndUpdateConfig(selectionResult, request, podKey)
			selectedNode := selectionResult.NodeName

			ctr.log.Info(fmt.Sprintf("konton_test: allocation = %v", allocation))

			selectedGPU := selectionResult.VGPUUUID
			// errCode 0: no error
			// errCode 1: node with nodeName is not initialized
			// errCode 2: vGPUID is not initialized or no DummyPod created;
			// errCode 3: resource exceed;
			// errCode 4: GPU is out of memory
			// errCode 5: No enough gpu client ports
			switch errCode {
			case 0:
				ctr.log.Info("the pod is successfully bound.")
				fstp2Pods[fstpName].PushBack(newPodName)
			case 1:
				return nil, errors.New("NodeNotInitialized")
			case 2:
				return nil, errors.New("Waiting4Dummy")
			case 3:
				err := fmt.Errorf("Compute Resource exceed!")
				utilruntime.HandleError(err)
				ctr.recorder.Event(fastpod, corev1.EventTypeWarning, ErrValueError, "Compute Resource exceed")
				return nil, err
			case 4:
				err := fmt.Errorf("Out of memory!")
				utilruntime.HandleError(err)
				ctr.recorder.Event(fastpod, corev1.EventTypeWarning, ErrValueError, "Out of memory")
				return nil, err
			case 5:
				err := fmt.Errorf("GPU Clients Port is full!")
				utilruntime.HandleError(err)
				ctr.recorder.Event(fastpod, corev1.EventTypeWarning, ErrValueError, "GPU Clients Port is not enough")
				return nil, err
			default:
				err := fmt.Errorf("Unknown Error")
				utilruntime.HandleError(err)
				ctr.recorder.Event(fastpod, corev1.EventTypeWarning, ErrValueError, "Unknown Error")
				return nil, err
			}

			// Create the new pod for the fastpod
			ctr.log.Info(fmt.Sprintf("starting to kube-create a new pod=%s for the fastpod=%s.", subpodName, key))

			podParams := &NewPodParams{
				PodName:      subpodName,
				SchedNode:    selectedNode,
				SchedvGPUID:  allocation.UUID,
				IsWarm:       false,
				BoundDevUUID: allocation.UUID,
				MPSConfig:    allocation.MPSConfig,
			}

			pod := ctr.newPod(fastpod, podParams)

			newpod, err := ctr.kubeClient.CoreV1().Pods(fastpodCopy.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
			if err != nil {
				ctr.log.Error(err, fmt.Sprintf("error when creating pod=%s for the fastpod=%s/%s. ", subpodName, fastpod.Namespace, fastpod.Name))
				ctr.log.Error(err, fmt.Sprintf("error: %s", err))

				//undo resource configuration

				if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
					return nil, nil
				}
				return nil, err
			}
			// KONTON_TODO
			(*fastpod.Status.BoundDeviceIDs)[newpod.Name] = selectedGPU

			if allocation.MPSConfig != nil && allocation.MPSConfig.FastPodMPSConfig != nil {
				(*fastpod.Status.GPUClientPort)[newpod.Name] = allocation.MPSConfig.FastPodMPSConfig.GpuClientPort
			}
			ctr.log.Info(fmt.Sprintf("finished creating pod = %s.", subpodName))
			return newpod, nil

		})

		// to do
		if skippedPodsNum := diff - successedNum; skippedPodsNum > 0 {
			ctr.log.Info(fmt.Sprintf("the controller does not create enough pod for fastpod=%s, created number:%d, falied number:%d.", key, successedNum, diff-successedNum))
			for i := 0; i < skippedPodsNum; i++ {

				//Decrement the expected number of creates because the informer won't observe this pod
				ctr.expectations.CreationObserved(fstpKey)
			}
		}
		return syncReplica, err
	} else if diff > 0 { // Too many Replicas, to delete pod to reconcile the spec
		ctr.log.V(2).Info(fmt.Sprintf("too many replicas for the fastpod ... \n need %d replicas, try to delete %d replicas", *fastpodCopy.Spec.Replicas, diff))
		podsToDelete := ctr.getPodsToDelete(existedPods, diff)
		syncReplica = true

		if podsToDelete == nil {
			ctr.log.V(2).Info(fmt.Sprintf("the number of pods=%d to delete exceeds the existed pods=%d.", diff, len(existedPods)))
		}

		ctr.expectations.ExpectDeletions(key, getPodKeys(podsToDelete))

		errCh := make(chan error, diff)

		var wg sync.WaitGroup
		wg.Add(diff)
		for _, pod := range podsToDelete {
			go func(targetPod *corev1.Pod, targetFastPod *fastpodv1.FaSTPod) {
				podCopy := targetPod.DeepCopy()
				defer func() {
					wg.Done()
					ctr.removePodFromList(fastpodCopy, podCopy)
				}()
				if err := ctr.kubeClient.CoreV1().Pods(targetPod.Namespace).Delete(ctx, targetPod.Name, metav1.DeleteOptions{}); err != nil {
					podKey := k8scontroller.PodKey(targetPod)
					ctr.expectations.DeletionObserved(key, podKey)
					if !apierrors.IsNotFound(err) {
						ctr.log.Info(fmt.Sprintf("failed to delete pod=%s of the fastpod=%s.", podKey, key))
						errCh <- err
					}
				}
				// update the status
				delete((*targetFastPod.Status.BoundDeviceIDs), targetPod.Name)
				delete((*targetFastPod.Status.GPUClientPort), targetPod.Name)
			}(pod, fastpod)
		}
		wg.Wait()
		select {
		case err := <-errCh:
			if err != nil {
				return syncReplica, err
			}
		default:
		}

	}

	return syncReplica, nil
}

func (ctr *Controller) reconcileResourceConfig(existedPods []*corev1.Pod, fastpod *fastpodv1.FaSTPod) error {
	reqName := fastpodv1.FaSTGShareGPUQuotaRequest
	limitName := fastpodv1.FaSTGShareGPUQuotaLimit
	smName := fastpodv1.FaSTGShareGPUSMPartition
	for _, pod := range existedPods {
		// configure the new resource via the fast-configurator
		nodeName := pod.Spec.NodeName
		vgpuID := pod.Annotations[fastpodv1.FaSTGShareVGPUUUID]
		node, ok := nodes[nodeName]
		if !ok {
			ctr.log.Error(nil, fmt.Sprintf("error failed to get node information for the pod %s.", pod.ObjectMeta.Name))
			continue
		}
		gpuInfo, ok := node.vGPUID2GPU[vgpuID]
		if !ok {
			ctr.log.Error(nil, fmt.Sprintf("error failed to get gpu information information for the pod %s.", pod.ObjectMeta.Name))
			continue
		}
		// Get the key of the pod
		key, err := cache.MetaNamespaceKeyFunc(pod)
		if err != nil {
			ctr.log.Error(err, fmt.Sprintf("error getting key: %v\n", err))
			continue
		}
		podreq, isFound := FindInQueue(key, gpuInfo.FastPodList)
		if !isFound {
			ctr.log.Error(nil, fmt.Sprintf("error failed to get pod information information for the pod %s.", pod.ObjectMeta.Name))
			continue
		}
		resourceValid := ctr.resourceValidityCheck(pod)
		if !resourceValid {
			ctr.log.Error(nil, fmt.Sprintf("error resource configuration for the pod %s is invalid.", pod.ObjectMeta.Name))
			continue
		}
		podreq.QtRequest, _ = strconv.ParseFloat(fastpod.ObjectMeta.Annotations[reqName], 64)
		podreq.QtLimit, _ = strconv.ParseFloat(fastpod.ObjectMeta.Annotations[limitName], 64)
		podreq.SMPartition, _ = strconv.Atoi(fastpod.ObjectMeta.Annotations[smName])

		ctr.updatePodsGPUConfig(nodeName, gpuInfo.UUID, gpuInfo.FastPodList)

		// update the spec.Annotation of the resource configuration for the pod of the fastpod
		podcpy := pod.DeepCopy()
		podcpy.Annotations[reqName] = fastpod.ObjectMeta.Annotations[reqName]
		podcpy.Annotations[limitName] = fastpod.ObjectMeta.Annotations[limitName]
		podcpy.Annotations[smName] = fastpod.ObjectMeta.Annotations[smName]
		_, err = ctr.kubeClient.CoreV1().Pods(podcpy.Namespace).Update(context.TODO(), podcpy, metav1.UpdateOptions{})
		if err != nil {
			tmperr := fmt.Errorf("error failed to update the resource annotation of the pod %s", podcpy.ObjectMeta.Name)
			ctr.log.Error(tmperr, "")
			return tmperr
		}
	}

	return nil
}

// slowStartBatch tries to call the provided function a total of 'count' times,
// starting slow to check for errors, then speeding up if calls succeed.
//
// It groups the calls into batches, starting with a group of initialBatchSize.
// Within each batch, it may call the function multiple times concurrently.
//
// If a whole batch succeeds, the next batch may get exponentially larger.
// If there are any failures in a batch, all remaining batches are skipped
// after waiting for the current batch to complete.
//
// It returns the number of successful calls to the function.
func slowStartbatch(count int, initailBatchSize int, fn func() (*corev1.Pod, error)) (int, error) {
	remaining := count
	successes := 0
	need2wait := 0
	for batchSize := integer.IntMin(remaining, initailBatchSize); batchSize > 0; batchSize = integer.IntMin(2*batchSize, remaining) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			func() {
				defer wg.Done()
				if pod, err := fn(); err != nil {
					if err.Error() != "Waiting4Dummy" {
						//continue
						errCh <- err
					}
					if pod == nil && err.Error() == "Waiting4Dummy" {
						need2wait++
					}

				}
			}()
		}

		wg.Wait()
		curSuccesses := batchSize - len(errCh) - need2wait
		successes += curSuccesses
		if len(errCh) > 0 {
			return successes, <-errCh
		}
		remaining -= batchSize
	}
	if need2wait > 0 {
		return successes, errors.New("Waiting4Dmmy")
	}
	return successes, nil
}

// enqueue the object to the controller's workqueue
func (ctr *Controller) enqueueFaSTPod(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	ctr.workqueue.Add(key)
}

func (ctr *Controller) handleDeletedFaSTPod(obj interface{}) {
	fastpod, ok := obj.(*fastpodv1.FaSTPod)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("handleDeletedFaSTPod: cannot parse object"))
		return
	}
	ctr.log.Info(fmt.Sprintf("starting to delete pods of fastpod %s/%s", fastpod.Namespace, fastpod.Name))
	go ctr.removeFaSTPodFromList(fastpod)
}

func (ctr *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleError(fmt.Errorf("Error decoding object, invalid type, type = %T", obj))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleError(fmt.Errorf("Error decoding object tombstone, invalid type, type = %T", tombstone.Obj))
			return
		}
		ctr.log.V(4).Info(fmt.Sprintf("recovered deleted object, resourcename: %s", object.GetName()))
	}

	// TODO logic to handle dummyPod if the status is "PodFailed"
	// dummyPod handling

	// enqueue FaSTPod key if it is a pod of a FaSTPod
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		if ownerRef.Kind != fastpodKind {
			return
		}
		ctr.log.Info(fmt.Sprintf("processing object = %s", object.GetName()))
		fastpod, err := ctr.fastpodsLister.FaSTPods(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			ctr.log.Info(fmt.Sprintf("ignoring orphaned object '%s' of fastpod '%s'", object.GetSelfLink(), ownerRef.Name))
			return
		}
		// klog.Infof("The pod=%s of the FaSTPod = %s is to be processed ...", object.GetName(), ownerRef.Name)
		ctr.log.Info("re-enqueue fastpod from pod update (func: handleObject)")
		ctr.enqueueFaSTPod(fastpod)
		return
	}
}

func (ctr *Controller) pendingInsurance(ticker *time.Ticker, done *chan bool) {
	for {
		select {
		case <-(*done):
			return
		case <-ticker.C:
			ctr.resourceChanged(nil)
		}
	}
}

func (ctr *Controller) resourceChanged(obj interface{}) {
	// push pending FaSTPods into workqueue
	ctr.pendingListMux.Lock()
	for p := ctr.pendingList.Front(); p != nil; p = p.Next() {
		ctr.workqueue.Add(p.Value)
	}
	ctr.pendingList.Init()
	ctr.pendingListMux.Unlock()
}
