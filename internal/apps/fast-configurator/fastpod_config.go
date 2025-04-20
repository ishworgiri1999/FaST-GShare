package fastconfigurator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	"k8s.io/klog/v2"
)

func initFiles() error {

	gcIpFile, err := os.Create(GPUClientsIPFile)
	if err != nil {
		klog.Errorf("Error Cannot create the GPUClientsIPFile = %s\n", GPUClientsIPFile)
		return err
	}

	st := os.Getenv(GPUClientsIPEnv) + "\n"
	gcIpFile.WriteString(st)
	gcIpFile.Sync()
	gcIpFile.Close()

	err = os.MkdirAll(FastSchedulerConfigDir, os.ModePerm)
	if err != nil {
		klog.Errorf("Error Cannot create the FastSchedulerConfigDir = %s\n", FastSchedulerConfigDir)
		return err
	}
	err = os.MkdirAll(GPUClientsPortConfigDir, os.ModePerm)
	if err != nil {
		klog.Errorf("Error Cannot create the GPUClientsPortConfigDir = %s\n", GPUClientsPortConfigDir)
		return err
	}

	return nil
}

func handleMsg(parsedConfig types.UpdatePodsGPUConfigMessage) {
	klog.Infof("Received message from fastpod-controller-manager: %v", parsedConfig)

	uuid := parsedConfig.GpuUUID

	klog.Infof("The gpu confiugration message, uuid=%s, data=%v\n", uuid, parsedConfig.PodGPURequests)
	confPath := filepath.Join(FastSchedulerConfigDir, uuid)
	confFile, err := os.Create(confPath)
	if err != nil {

		klog.Errorf("Error failed to create the fast scheduler resource configuration file: %s\n.", confPath)
		klog.Errorf("Error :%v", err)
	}

	gcPortFilePath := filepath.Join(GPUClientsPortConfigDir, uuid)
	gcPortFile, err := os.Create(gcPortFilePath)
	if err != nil {
		klog.Errorf("Error failed to create the gpu clients' port configuration file: %s\n.", gcPortFilePath)
	}

	confFile.WriteString(fmt.Sprintf("%d\n", len(parsedConfig.PodGPURequests)))

	for _, podReq := range parsedConfig.PodGPURequests {
		fastSchedConf := podGPUSchedulerConfigFromat(podReq)
		confFile.WriteString(fastSchedConf)
	}
	confFile.Sync()
	confFile.Close()
	gcPortFile.WriteString(fmt.Sprintf("%d\n", len(parsedConfig.PodGPURequests)))
	for _, podReq := range parsedConfig.PodGPURequests {
		gpuClientsPort := PodGPUClientsPortConfigFormat(podReq)
		gcPortFile.WriteString(gpuClientsPort)
	}

	gcPortFile.Sync()
	gcPortFile.Close()
}

func podGPUSchedulerConfigFromat(podReq types.PodGPURequest) string {
	return fmt.Sprintf("%s %f %f %d %d\n", podReq.Key, podReq.QtRequest, podReq.QtLimit, podReq.SMPartition, podReq.Memory)
}

func PodGPUClientsPortConfigFormat(podReq types.PodGPURequest) string {
	return fmt.Sprintf("%s %d\n", podReq.Key, podReq.GPUClientPort)
}
