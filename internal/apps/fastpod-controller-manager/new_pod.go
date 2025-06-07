package fastpodcontrollermanager

import (
	"fmt"
	"strconv"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
	"github.com/KontonGu/FaST-GShare/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

type NewPodParams struct {
	FaSTPod      *fastpodv1.FaSTPod
	IsWarm       bool
	BoundDevUUID string
	SchedNode    string
	SchedvGPUID  string
	PodName      string
	MPSConfig    *MPSConfig
}

type MPSConfig struct {
	LogDirectory     string
	PipeDirectory    string
	FastPodMPSConfig *FastPodMPSConfig
}

type FastPodMPSConfig struct {
	SchedulerIP            string
	GpuClientPort          int
	ActiveThreadPercentage int
}

// newPod create a new pod specification based on the given information for the FaSTPod
func (ctr *Controller) newPod(fastpod *fastpodv1.FaSTPod, params *NewPodParams) *corev1.Pod {
	specCopy := fastpod.Spec.PodSpec.DeepCopy()
	specCopy.NodeName = params.SchedNode

	labelCopy := makeLabels(fastpod)
	annotationCopy := make(map[string]string, len(fastpod.ObjectMeta.Annotations)+5)
	for key, val := range fastpod.ObjectMeta.Annotations {
		annotationCopy[key] = val
	}

	// TODO pre-warm setting for cold start issues, currently TODO...
	isWarm := params.IsWarm
	if isWarm {
		annotationCopy[FastGShareWarm] = "true"
	} else {
		annotationCopy[FastGShareWarm] = "false"
	}

	smPartition := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUSMPartition]

	for i := range specCopy.Containers {
		ctn := &specCopy.Containers[i]
		ctn.Env = append(ctn.Env,
			corev1.EnvVar{
				Name:  "NVIDIA_VISIBLE_DEVICES",
				Value: params.BoundDevUUID,
			},
			corev1.EnvVar{
				Name:  "CUDA_VISIBLE_DEVICES",
				Value: params.BoundDevUUID,
			},
			corev1.EnvVar{
				Name:  "NVIDIA_DRIVER_CAPABILITIES",
				Value: "compute,utility",
			},
		)

		// Add MPS related env vars only if MPSConfig exists
		if params.MPSConfig != nil {
			ctn.Env = append(ctn.Env,
				corev1.EnvVar{
					Name:  "CUDA_MPS_PIPE_DIRECTORY",
					Value: "/tmp/mps",
				},
				corev1.EnvVar{
					Name:  "CUDA_MPS_LOG_DIRECTORY",
					Value: "/tmp/mps_log",
				},
			)
			if params.MPSConfig.FastPodMPSConfig != nil {

				ctn.Env = append(ctn.Env,
					corev1.EnvVar{
						Name:  "CUDA_MPS_ACTIVE_THREAD_PERCENTAGE",
						Value: smPartition,
					},
					corev1.EnvVar{
						// the scheduler IP is not necessary since the hooked containers get it from /fastpod/library/GPUClientsIP.txt
						Name:  "SCHEDULER_IP",
						Value: params.MPSConfig.FastPodMPSConfig.SchedulerIP,
					},
					corev1.EnvVar{
						Name:  "GPU_CLIENT_PORT",
						Value: fmt.Sprintf("%d", params.MPSConfig.FastPodMPSConfig.GpuClientPort),
					},
					corev1.EnvVar{
						Name:  "LD_PRELOAD",
						Value: FaSTPodLibraryDir + "/libfast.so.1",
					},
				)
			}
		}

		ctn.Env = append(ctn.Env,
			corev1.EnvVar{
				Name:  "POD_NAME",
				Value: fmt.Sprintf("%s/%s", fastpod.ObjectMeta.Namespace, params.PodName),
			},
		)

		ctn.VolumeMounts = append(ctn.VolumeMounts,
			corev1.VolumeMount{
				Name:      "fastpod-lib",
				MountPath: FaSTPodLibraryDir,
			},
		)

		if params.MPSConfig != nil {
			ctn.VolumeMounts = append(ctn.VolumeMounts,
				corev1.VolumeMount{
					Name:      "nvidia-mps-tmp",
					MountPath: "/tmp/mps",
				},
				corev1.VolumeMount{
					Name:      "nvidia-mps-tmp-log",
					MountPath: "/tmp/mps_log",
				},
			)
		}
		ctn.ImagePullPolicy = fastpod.Spec.PodSpec.Containers[0].ImagePullPolicy
	}

	specCopy.Volumes = append(specCopy.Volumes,
		corev1.Volume{
			Name: "fastpod-lib",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: FaSTPodLibraryDir,
				},
			},
		},
	)

	if params.MPSConfig != nil {
		specCopy.Volumes = append(specCopy.Volumes,
			corev1.Volume{
				Name: "nvidia-mps-tmp",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: params.MPSConfig.PipeDirectory,
					},
				},
			},
			corev1.Volume{
				Name: "nvidia-mps-tmp-log",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: params.MPSConfig.LogDirectory,
					},
				},
			},
		)
	}

	annotationCopy[fastpodv1.FaSTGShareGPUQuotaRequest] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest]
	annotationCopy[fastpodv1.FaSTGShareGPUQuotaLimit] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit]
	annotationCopy[fastpodv1.FaSTGShareGPUMemory] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory]
	annotationCopy[fastpodv1.FaSTGShareVGPUUUID] = params.SchedvGPUID
	annotationCopy[fastpodv1.FastGshareAllocationType] = fastpod.ObjectMeta.Annotations[fastpodv1.FastGshareAllocationType]
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.PodName,
			Namespace: fastpod.ObjectMeta.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(fastpod, schema.GroupVersionKind{
					Group:   fastpodv1.SchemeGroupVersion.Group,
					Version: fastpodv1.SchemeGroupVersion.Version,
					Kind:    fastpodKind,
				}),
			},
			Annotations: annotationCopy,
			Labels:      labelCopy,
		},
		Spec: corev1.PodSpec{
			NodeName:   params.SchedNode,
			Containers: specCopy.Containers,
			Volumes:    specCopy.Volumes,
			HostIPC:    true,
			//InitContainers: []corev1.Container{},
		},
	}
}

func getPodRequestFromPod(fastpod *fastpodv1.FaSTPod) (*ResourceRequest, error) {

	resourceRequest := &ResourceRequest{
		podKey: fmt.Sprintf("%s/%s", fastpod.Namespace, fastpod.Name),
	}
	var fastPodRequirements *FastPodRequirements

	quota := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest]
	quotaLimit := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit]
	gpuMemory := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory]
	allocationType := fastpod.ObjectMeta.Annotations[fastpodv1.FastGshareAllocationType]
	smPartition := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUSMPartition]
	smValue := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUSMValue]

	node := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareNodeName]
	vgpuType := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareVGPUType]
	vgpuUUID := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareVGPUUUID]

	allocationTypeValue := types.GetAllocationType(allocationType)

	gpuMemoryValue, err := strconv.ParseInt(gpuMemory, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPU memory: %v", err)
	}

	resourceRequest.Memory = gpuMemoryValue
	resourceRequest.AllocationType = allocationTypeValue

	if len((node)) > 0 {
		resourceRequest.RequestedNode = &node
	}
	if len((vgpuType)) > 0 {
		resourceRequest.RequestedGPUType = &vgpuType
	}
	if len((vgpuUUID)) > 0 {
		resourceRequest.RequestGPUUUID = &vgpuUUID
	}

	if allocationTypeValue == types.AllocationTypeFastPod {
		quotaValue, err := strconv.ParseFloat(quota, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GPU quota request: %v", err)
		}
		quotaLimitValue, err := strconv.ParseFloat(quotaLimit, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GPU quota limit: %v", err)
		}

		smPartitionValue, err := strconv.Atoi(smPartition)

		if err != nil {
			return nil, fmt.Errorf("failed to parse SM partition: %v", err)
		}

		fastPodRequirements = &FastPodRequirements{
			QuotaReq:    quotaValue,
			QuotaLimit:  quotaLimitValue,
			SMPartition: int(smPartitionValue),
		}

		resourceRequest.FastPodRequirements = fastPodRequirements
		resourceRequest.SMPercentage = &smPartitionValue

	}

	if allocationTypeValue == types.AllocationTypeExclusive {
		//one is enough
		smValueInt, err1 := strconv.Atoi(smValue)

		smPartitionValue, err2 := strconv.Atoi(smPartition)

		if err1 != nil && err2 != nil {
			return nil, fmt.Errorf("failed to parse SM value: %v", err1)
		}

		klog.Infof("SM value: %d, SM partition value: %d", smValueInt, smPartitionValue)

		if smValueInt > 0 {
			resourceRequest.SMRequest = &smValueInt
		} else {
			resourceRequest.SMPercentage = &smPartitionValue
		}
	}

	return resourceRequest, nil
}

func validatePodRequest(request *ResourceRequest) (bool, error) {

	if request.AllocationType == types.AllocationTypeNone {
		return false, fmt.Errorf("AllocationType is None")
	}

	if request.FastPodRequirements != nil {
		if request.FastPodRequirements.QuotaLimit > 1.0 || request.FastPodRequirements.QuotaLimit < 0.0 {
			return false, fmt.Errorf("invalid quota limitation value: %f", request.FastPodRequirements.QuotaLimit)
		}

		if request.FastPodRequirements.QuotaReq > 1.0 || request.FastPodRequirements.QuotaReq < 0.0 {
			return false, fmt.Errorf("invalid quota request value: %f", request.FastPodRequirements.QuotaReq)
		}

		if request.FastPodRequirements.SMPartition < 0 || request.FastPodRequirements.SMPartition > 100 {
			return false, fmt.Errorf("invalid SM partition value: %d", request.FastPodRequirements.SMPartition)
		}
	}

	if request.AllocationType == types.AllocationTypeExclusive {
		if request.SMRequest == nil && request.SMPercentage == nil {
			return false, fmt.Errorf("either SMRequest or SMPercentage must be provided for exclusive allocation")
		}

		if request.SMRequest != nil && *request.SMRequest <= 0 {
			return false, fmt.Errorf("SMRequest must be greater than 0")
		}

		if request.SMPercentage != nil && (*request.SMPercentage < 0 || *request.SMPercentage > 100) {
			return false, fmt.Errorf("SMPercentage must be between 0 and 100")
		}

	}

	if request.Memory < 0 {
		return false, fmt.Errorf("invalid memory value: %d", request.Memory)
	}

	return true, nil
}
