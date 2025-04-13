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
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	"k8s.io/klog/v2"
)

const MPS_START_PATH = "/tmp/mps_"

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

func Run(controllerManagerAddress string, mps bool) {
	klog.Infof("Starting FaST-GShare configurator...")

	// err := initFiles()

	// if err != nil {
	// 	klog.Fatalf("Error failed to initialize files: %v", err)
	// 	return
	// }

	server, err := NewServer("5001")
	if err != nil {
		klog.Fatalf("Error failed to create gRPC server: %v", err)
		return
	}
	klog.Infof("gRPC server started on port 5001")

	defer server.Stop()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel to receive connection closed notifications
	connClosedChan := make(chan struct{})

	// Connection establishment function that can be called for both initial connection and reconnection
	connectToController := func() (net.Conn, *bufio.Reader, error) {
		klog.Infof("Trying to connect to controller-manager....., server IP:Port = %s\n", controllerManagerAddress)
		retryCount := 0
		var conn net.Conn
		var err error

		for retryCount < MaxConnRetries {
			conn, err = net.Dial("tcp", controllerManagerAddress)
			if err != nil {
				klog.Errorf("Error Failed to connect (attempt %d/%d) the device-controller-manager, IP:Port = %s, : %v .",
					retryCount+1, MaxConnRetries, controllerManagerAddress, err)
				klog.Errorf("Retrying in %d seconds...", RetryItv)
				retryCount++
				select {
				case <-sigChan:
					klog.Info("Received signal during connection attempt, aborting")
					return nil, nil, err
				case <-time.After(RetryItv * time.Second):
					continue
				}
			}
			break
		}

		if retryCount+1 >= MaxConnRetries {
			return nil, nil, fmt.Errorf("maximum connection retries exceeded")
		}

		// Connection established
		reader := bufio.NewReader(conn)
		return conn, reader, nil
	}

	// Initial connection
	conn, reader, err := connectToController()
	if err != nil {
		klog.Fatalf("Failed to establish initial connection: %v", err)
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Error Cannot get hostname. \n")
		panic(err)
	}

	klog.Info("The connection to the device-controller-manager succeed.")

	// Send hello message
	helloMessage := types.ConfiguratorNodeHelloMessage{
		Hostname: hostname,
		GrpcPort: 5001,
	}
	klog.Infof("Sending hello message to the device-controller-manager with hostname: %s\n", hostname)
	b, err := types.EncodeToByte(helloMessage)

	klog.Infof("The encoded hello message: %v\n %d", b, len(b))
	if err != nil {
		klog.Fatalf("Error failed to encode ConfiguratorNodeHelloMessage: %v", err)
	}

	_, err = conn.Write(b)
	if err != nil {
		klog.Error("Error failed to write msg: %v", err)
		conn.Close()
		return
	}

	configMsg, err := reader.ReadBytes('\n')
	if err != nil {
		klog.Errorf("Error while Receiving Msg from fastpod-controller-manager: %v", err)
		conn.Close()
		return
	}

	var ack types.ConfiguratorNodeAckMessage
	err = types.DecodeFromByte(configMsg, &ack)

	if err != nil {
		klog.Fatalf("Error failed to decode ConfiguratorNodeAckMessage: %v", err)
		conn.Close()
		return
	}
	if !ack.Ok {
		klog.Fatalf("Error failed to receive ack message from device-controller-manager: %v", err)
		conn.Close()
		return
	}

	klog.Infof("Received ack message from device-controller-manager: %v", ack)

	// Start heartbeat
	nodeHeartbeater := time.NewTicker(time.Second * time.Duration(HeartbeatItv))

	// Start a goroutine to handle reconnection
	go func() {
		// Start the heartbeat goroutine and get the connection closed channel
		heartbeatDone := make(chan struct{})
		go func() {
			sendNodeHeartbeats(conn, nodeHeartbeater.C, heartbeatDone)
		}()

		// Wait for either a signal or connection closure
		select {
		case <-sigChan:
			klog.Info("Received shutdown signal, closing connection and exiting")
			conn.Close()
			return
		case <-heartbeatDone:
			klog.Warning("Connection to controller-manager lost, attempting to reconnect")

			// Cleanup old connection and ticker
			conn.Close()
			nodeHeartbeater.Stop()

			// Try to reconnect
			reconnConn, reconnReader, reconnErr := connectToController()
			if reconnErr != nil {
				klog.Errorf("Failed to reconnect: %v", reconnErr)
				close(connClosedChan) // Signal main goroutine to exit
				return
			}

			klog.Info("Successfully reconnected to controller-manager")
			conn = reconnConn
			reader = reconnReader

			// Send hello message again
			_, err = conn.Write(b) // Reuse the hello message from before
			if err != nil {
				klog.Errorf("Failed to send hello message after reconnect: %v", err)
				conn.Close()
				close(connClosedChan)
				return
			}

			// Wait for ack again
			configMsg, err = reader.ReadBytes('\n')
			if err != nil {
				klog.Errorf("Error reading ack after reconnect: %v", err)
				conn.Close()
				close(connClosedChan)
				return
			}

			err = types.DecodeFromByte(configMsg, &ack)
			if err != nil || !ack.Ok {
				klog.Errorf("Failed to receive valid ack after reconnect: %v", err)
				conn.Close()
				close(connClosedChan)
				return
			}

			// Restart heartbeat
			nodeHeartbeater = time.NewTicker(time.Second * time.Duration(HeartbeatItv))
			go func() {
				sendNodeHeartbeats(conn, nodeHeartbeater.C, heartbeatDone)
			}()
		}
	}()

	// Wait for either a shutdown signal or notification that the connection is permanently closed
	select {
	case <-sigChan:
		klog.Infof("Received signal, shutting down")
	case <-connClosedChan:
		klog.Infof("Connection permanently closed, shutting down")
	}

	// Cleanup
	if conn != nil {
		conn.Close()
	}
}

func RunOld(controllerManagerAddress string, mps bool) {
	klog.Infof("Starting FaST-GShare configurator...")

	server, err := NewServer("5001")
	if err != nil {
		klog.Fatalf("Error failed to create gRPC server: %v", err)
		return
	}
	klog.Infof("gRPC server started on port 5001")

	defer server.Stop()

	manager, err := NewResourceManager()

	if err != nil {
		klog.Fatalf("Error failed to initialize ResourceManager: %v", err)
		return
	}

	//filter out non-usable GPUs, mig parents are not usable
	gpus := manager.getAvailableVirtualResources()

	log.Printf("count of available GPUs: %d\n", len(manager.PhysicalGPUs))

	if len(gpus) == 0 {
		klog.Fatalf("No usable GPUs found.")
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

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
			select {
			case <-sigChan:
				klog.Info("Received signal during connection attempt, aborting")
				return
			case <-time.After(RetryItv * time.Second):
				continue
			}

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

	nodeHeartbeater := time.NewTicker(time.Second * time.Duration(HeartbeatItv))
	go sendNodeHeartbeats(conn, nodeHeartbeater.C, nil)

	go func() {
		<-sigChan
		conn.Close()
		klog.Infof("Received signal, shutting down fast-configurator")
	}()

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

func registerGPUDevices(conn net.Conn, gpus []*VirtualGPU) {

	gpu_num := len(gpus)
	var buf bytes.Buffer

	var gpuinfo types.GPURegisterMessage = types.GPURegisterMessage{
		GPU: make([]types.VirtualGPU, gpu_num),
	}

	for i := 0; i < gpu_num; i++ {

		// uuid := gpus[i].UUID
		// memsize := gpus[i].Memory
		// oriGPUType := gpus[i].Name
		// gpuTypeName := strings.Split(oriGPUType, " ")[1]

		// uuidWithType := uuid + "_" + gpuTypeName
		// buf.WriteString(uuidWithType + ":")
		// buf.WriteString(strconv.FormatUint(memsize, 10))
		// buf.WriteString(",")

		var provisionedGPU *types.GPU

		if gpus[i].ProvisionedGPU != nil {
			provisionedGPU = &types.GPU{
				UUID:     gpus[i].ProvisionedGPU.UUID,
				TypeName: gpus[i].ProvisionedGPU.Name,
				Memory:   gpus[i].ProvisionedGPU.MemoryBytes,
			}

			gpu := types.VirtualGPU{
				MemoryBytes:         gpus[i].MemoryBytes,
				MultiProcessorCount: gpus[i].MultiProcessorCount,
				IsProvisioned:       gpus[i].IsProvisioned,
				InUse:               gpus[i].InUse,
				ProvisionedGPU:      provisionedGPU,
			}

			gpuinfo.GPU[i] = gpu
		}

	}

	klog.Infof("Registering %d GPU devices.", gpu_num)

	b, err := types.EncodeToByte(gpuinfo)
	if err != nil {
		klog.Fatalf("Error failed to encode GPURegisterMessage: %v", err)
	}
	klog.Infof("Sucessfully get GPU devices, <uuid>:<memory>,... = %s\n", buf.String())
	conn.Write(b)

}

func sendNodeHeartbeats(conn net.Conn, heartTick <-chan time.Time, connectionClosed chan struct{}) {
	klog.Infof("Send node heartbeat to fastpod-controller-manager: %s\n", time.Now().String())
	heartbeat := types.ConfiguratorHeartbeatMessage{
		Alive: true,
	}
	beat, err := types.EncodeToByte(heartbeat)

	if err != nil {
		klog.Fatalf("Error failed to encode ConfiguratorHeartbeatMessage: %v", err)
	}

	// Send first heartbeat
	err = writeMsgToConn(conn, beat)
	if err != nil {
		klog.Errorf("Error failed to write initial heartbeat msg: %v", err)
		if connectionClosed != nil {
			close(connectionClosed)
		}
		return
	}
	klog.Info("First heartbeat sent to fastpod-controller-manager.")

	// Counter for consecutive failures
	consecutiveFailures := 0

	// Maximum number of failures before giving up
	const maxConsecutiveFailures = 3

	for {
		select {
		case <-heartTick:
			klog.Infof("Send node heartbeat to fastpod-controller-manager: %s\n", time.Now().String())
			e := writeMsgToConn(conn, beat)
			if e != nil {
				consecutiveFailures++
				klog.Errorf("Error failed to write heartbeat msg (%d/%d failures): %v",
					consecutiveFailures, maxConsecutiveFailures, e)

				// Check if we should give up
				if consecutiveFailures >= maxConsecutiveFailures {
					klog.Errorf("Connection to controller-manager appears to be closed after %d failed attempts",
						consecutiveFailures)
					if connectionClosed != nil {
						close(connectionClosed)
					}
					return
				}
			} else {
				// Reset failure counter on successful send
				consecutiveFailures = 0
			}
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
