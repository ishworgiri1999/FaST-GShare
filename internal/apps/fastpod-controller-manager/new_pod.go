package fastpodcontrollermanager

import (
	"fmt"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	LogDirectory           string
	PipeDirectory          string
	ActiveThreadPercentage int
	FastPodMPSConfig       *FastPodMPSConfig
}

type FastPodMPSConfig struct {
	SchedulerIP   string
	GpuClientPort int
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
				corev1.EnvVar{
					Name:  "CUDA_MPS_ACTIVE_THREAD_PERCENTAGE",
					Value: smPartition,
				},
			)
			if params.MPSConfig.FastPodMPSConfig != nil {
				ctn.Env = append(ctn.Env,
					corev1.EnvVar{
						// the scheduler IP is not necessary since the hooked containers get it from /fastpod/library/GPUClientsIP.txt
						Name:  "SCHEDULER_IP",
						Value: params.MPSConfig.FastPodMPSConfig.SchedulerIP,
					},
					corev1.EnvVar{
						Name:  "GPU_CLIENT_PORT",
						Value: fmt.Sprintf("%d", params.MPSConfig.FastPodMPSConfig.GpuClientPort),
						// corev1.EnvVar{
						// 	Name: "LD_PRELOAD",
						// 	// Value: FaSTPodLibraryDir + "/libfast.so.1_with_debug",
						// },
					})
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
			corev1.VolumeMount{
				Name:      "nvidia-mps-tmp",
				MountPath: "/tmp/mps",
			},
			corev1.VolumeMount{
				Name:      "nvidia-mps-tmp-log",
				MountPath: "/tmp/mps_log",
			},
		)
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
	annotationCopy[fastpodv1.FaSTGShareGPUQuotaRequest] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest]
	annotationCopy[fastpodv1.FaSTGShareGPUQuotaLimit] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit]
	annotationCopy[fastpodv1.FaSTGShareGPUMemory] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory]
	annotationCopy[fastpodv1.FaSTGShareVGPUID] = params.SchedvGPUID

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
