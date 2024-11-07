/*
 * Created/Modified on Mon May 27 2024
 *
 * Author: KontonGu (Jianfeng Gu)
 * Copyright (c) 2024 TUM - CAPS Cloud
 * Licensed under the Apache License, Version 2.0 (the "License")
 */

package fastconfigurator

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"path/filepath"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"k8s.io/klog/v2"
)

var (
	GPUClientsIPEnv = "FaSTPod_GPUClientsIP"
)

const (
	GPUClientsIPFile        = "/fastpod/library/GPUClientsIP.txt"
	FastSchedulerConfigDir  = "/fastpod/scheduler/config"
	GPUClientsPortConfigDir = "/fastpod/scheduler/gpu_clients"
	HeartbeatItv            = 60
	MaxConnRetries          = 15
	RetryItv                = 10
)

func Run(controllerManagerAddress string) {
	gcIpFile, err := os.Create(GPUClientsIPFile)
	if err != nil {
		klog.Errorf("Error Cannot create the GPUClientsIPFile = %s\n", GPUClientsIPFile)
	}

	st := os.Getenv(GPUClientsIPEnv) + "\n"
	gcIpFile.WriteString(st)
	gcIpFile.Sync()
	gcIpFile.Close()
	klog.Infof("Node Daemon, GPUClientsIP = %s.", st)

	hostname, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Error Cannot get hostname. \n")
		panic(err)
	}

	os.MkdirAll(FastSchedulerConfigDir, os.ModePerm)
	os.MkdirAll(GPUClientsPortConfigDir, os.ModePerm)

	klog.Infof("Trying to connet controller-manager....., server IP:Port = %s\n", controllerManagerAddress)
	retryCount := 0
	var conn net.Conn
	for retryCount < MaxConnRetries {
		conn, err = net.Dial("tcp", controllerManagerAddress)
		if err != nil {
			klog.Errorf("Error Failed to connect (attempt %d/%d) the device-controller-manager, IP:Port = %s, : %v .", retryCount+1, MaxConnRetries, controllerManagerAddress, err)
			klog.Errorf("Retrying in %d seconds...", RetryItv)
			retryCount++
			time.Sleep(RetryItv * time.Second)
			continue
		}
		break
	}
	if retryCount+1 >= MaxConnRetries {
		panic(err)
	}

	klog.Info("The connection to the device-controller-manager succeed.")
	reader := bufio.NewReader(conn)
	helloMessage := types.ConfiguratorNodeHelloMessage{
		Hostname: hostname,
	}
	klog.Infof("Sending hello message to the device-controller-manager with hostname: %s\n", hostname)
	b, err := types.EncodeToByte(helloMessage)

	klog.Infof("The encoded hello message: %v\n %d", b, len(b))
	if err != nil {
		klog.Fatalf("Error failed to encode ConfiguratorNodeHelloMessage: %v", err)
	}
	writeMsgToConn(conn, b)

	registerGPUDevices(conn)

	nodeHeartbeater := time.NewTicker(time.Second * time.Duration(HeartbeatItv))
	go sendNodeHeartbeats(conn, nodeHeartbeater.C)

	recvMsgAndWriteConfig(reader)
}

func writeMsgToConn(conn net.Conn, b []byte) error {
	_, err := conn.Write(b)
	if err != nil {
		klog.Errorf("Error failed to write msg: %v", err)
		return err
	}
	return nil
}

func registerGPUDevices(conn net.Conn) {
	gpu_num, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		klog.Fatalf("Unable to get device count: %v", nvml.ErrorString(ret))
	}
	var buf bytes.Buffer

	var gpuinfo types.GPURegisterMessage = types.GPURegisterMessage{
		GPU: make([]types.GPU, gpu_num),
	}

	for i := 0; i < gpu_num; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			klog.Fatalf("Unable to get device at index %d: %v", i, nvml.ErrorString(ret))
		}

		meminfo, ret := device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			klog.Fatalf("Unable to get GPU memory %d: %v", i, nvml.ErrorString(ret))
		}
		memsize := meminfo.Total

		uuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			klog.Fatalf("Unable to get uuid of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		oriGPUType, ret := device.GetName()
		if ret != nvml.SUCCESS {
			klog.Fatalf("Unable to get name of device at index %d: %v", i, nvml.ErrorString(ret))
		}
		gpuTypeName := strings.Split(oriGPUType, " ")[1]

		uuidWithType := uuid + "_" + gpuTypeName
		buf.WriteString(uuidWithType + ":")
		buf.WriteString(strconv.FormatUint(memsize, 10))
		buf.WriteString(",")

		gpu := types.GPU{
			UUID:     uuid,
			TypeName: gpuTypeName,
			Memory:   memsize,
		}
		gpuinfo.GPU[i] = gpu
	}

	b, err := types.EncodeToByte(gpuinfo)
	if err != nil {
		klog.Fatalf("Error failed to encode GPURegisterMessage: %v", err)
	}
	klog.Infof("Sucessfully get GPU devices, <uuid>:<memory>,... = %s\n", buf.String())
	conn.Write(b)

}

func sendNodeHeartbeats(conn net.Conn, heartTick <-chan time.Time) {
	klog.Infof("Send node heartbeat to fastpod-controller-manager: %s\n", time.Now().String())
	heartbeat := types.ConfiguratorHeartbeatMessage{
		Alive: true,
	}
	beat, err := types.EncodeToByte(heartbeat)

	if err != nil {
		klog.Fatalf("Error failed to encode ConfiguratorHeartbeatMessage: %v", err)
	}

	err = writeMsgToConn(conn, beat)
	if err != nil {
		klog.Infof("Error failed to write heartbeat msg: %v", err)
	}
	klog.Info("First heartbeat sent to fastpod-controller-manager.")
	for {
		<-heartTick
		klog.Infof("Send node heartbeat to fastpod-controller-manager: %s\n", time.Now().String())
		e := writeMsgToConn(conn, beat)
		if e != nil {
			klog.Infof("Error failed to write heartbeat msg: %v", e)
		}
	}
}

func recvMsgAndWriteConfig(reader *bufio.Reader) {
	klog.Infof("Receiving Resource and Port Configuration from fastpod-controller-manager. \n")
	for {
		configMsg, err := reader.ReadBytes('\n')
		if err != nil {
			klog.Errorf("Error while Receiving Msg from fastpod-controller-manager")
			return
		}

		var parsedConfig types.UpdatePodsGPUConfigMessage
		err = types.DecodeFromByte(configMsg, &parsedConfig)
		if err != nil {
			klog.Errorf("Error failed to decode UpdatePodsGPUConfigMessage: %v", err)
			return
		}

		handleMsg(parsedConfig)
	}
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
