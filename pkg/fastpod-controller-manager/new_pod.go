package fastpodcontrollermanager

import (
	"fmt"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// newPod create a new pod specification based on the given information for the FaSTPod
func (ctr *Controller) newPod(fastpod *fastpodv1.FaSTPod, isWarm bool, schedIP string, gpuClientPort int, boundDevUUID, schedNode, schedvGPUID, podName string) *corev1.Pod {
	specCopy := fastpod.Spec.PodSpec.DeepCopy()
	specCopy.NodeName = schedNode

	labelCopy := makeLabels(fastpod)
	annotationCopy := make(map[string]string, len(fastpod.ObjectMeta.Annotations)+5)
	for key, val := range fastpod.ObjectMeta.Annotations {
		annotationCopy[key] = val
	}

	// TODO pre-warm setting for cold start issues, currently TODO...
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
				Value: boundDevUUID,
			},
			corev1.EnvVar{
				Name:  "NVIDIA_DRIVER_CAPABILITIES",
				Value: "compute,utility",
			},
			corev1.EnvVar{
				Name:  "CUDA_MPS_PIPE_DIRECTORY",
				Value: "/fastpod/mps/tmp",
			},
			corev1.EnvVar{
				Name:  "CUDA_MPS_LOG_DIRECTORY",
				Value: "/fastpod/mps/log",
			},
			corev1.EnvVar{
				Name:  "CUDA_MPS_ACTIVE_THREAD_PERCENTAGE",
				Value: smPartition,
			},
			// corev1.EnvVar{
			// 	Name:  "LD_PRELOAD",
			// 	Value: FaSTPodLibraryDir + "/libfast_new.so.1",
			// },
			corev1.EnvVar{
				// the scheduler IP is not necessary since the hooked containers get it from /fastpod/library/GPUClientsIP.txt
				Name:  "SCHEDULER_IP",
				Value: schedIP,
			},
			corev1.EnvVar{
				Name:  "GPU_CLIENT_PORT",
				Value: fmt.Sprintf("%d", gpuClientPort),
			},
			corev1.EnvVar{
				Name:  "POD_NAME",
				Value: fmt.Sprintf("%s/%s", fastpod.ObjectMeta.Namespace, podName),
			},
		)
		ctn.VolumeMounts = append(ctn.VolumeMounts,
			corev1.VolumeMount{
				Name:      "fastpod-lib",
				MountPath: FaSTPodLibraryDir,
			},
			corev1.VolumeMount{
				Name:      "nvidia-mps",
				MountPath: "/fastpod/mps",
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
			Name: "nvidia-mps",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/fastpod/mps",
				},
			},
		},
	)
	annotationCopy[fastpodv1.FaSTGShareGPUQuotaRequest] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest]
	annotationCopy[fastpodv1.FaSTGShareGPUQuotaLimit] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit]
	annotationCopy[fastpodv1.FaSTGShareGPUMemory] = fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory]
	annotationCopy[fastpodv1.FaSTGShareVGPUID] = schedvGPUID

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
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
			NodeName:   schedNode,
			Containers: specCopy.Containers,
			Volumes:    specCopy.Volumes,
			HostIPC:    true,
			//InitContainers: []corev1.Container{},
		},
	}
}
