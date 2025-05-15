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

func maintainConnection(
	name string,
	address string,
	helloMsg types.ConfiguratorNodeHelloMessage,
	sigChan <-chan os.Signal,
	handleMsgFunc func(*bufio.Reader),
) {
	for {
		klog.Infof("[%s] Attempting to connect to %s", name, address)
		var conn net.Conn
		var reader *bufio.Reader
		var err error
		for {
			select {
			case <-sigChan:
				klog.Infof("[%s] Received signal during connection attempt, aborting goroutine", name)
				return
			default:
			}
			conn, err = net.Dial("tcp", address)
			if err != nil {
				klog.Errorf("[%s] Connection failed: %v. Retrying in %d seconds...", name, err, RetryItv)
				time.Sleep(RetryItv * time.Second)
				continue
			}
			reader = bufio.NewReader(conn)
			break
		}

		// Send hello message
		b, err := types.EncodeToByte(helloMsg)
		if err != nil {
			klog.Errorf("[%s] Failed to encode hello message: %v", name, err)
			conn.Close()
			continue
		}
		_, err = conn.Write(b)
		if err != nil {
			klog.Errorf("[%s] Failed to send hello message: %v", name, err)
			conn.Close()
			continue
		}

		// Wait for ack
		ackMsg, err := reader.ReadBytes('\n')
		if err != nil {
			klog.Errorf("[%s] Failed to read ack: %v", name, err)
			conn.Close()
			continue
		}
		var ack types.ConfiguratorNodeAckMessage
		err = types.DecodeFromByte(ackMsg, &ack)
		if err != nil || !ack.Ok {
			klog.Errorf("[%s] Invalid ack: %v", name, err)
			conn.Close()
			continue
		}
		klog.Infof("[%s] Connection established and ack received.", name)

		// Start heartbeat
		heartTicker := time.NewTicker(time.Second * time.Duration(HeartbeatItv))
		heartbeat := types.ConfiguratorHeartbeatMessage{Alive: true}
		beat, _ := types.EncodeToByte(heartbeat)
		heartbeatFailures := 0
		const maxHeartbeatFailures = 3

		// Start message handler if provided
		done := make(chan struct{})
		if handleMsgFunc != nil {
			go func() {
				handleMsgFunc(reader)
				close(done)
			}()
		}

		// Heartbeat loop
	heartbeatLoop:
		for {
			select {
			case <-sigChan:
				klog.Infof("[%s] Received signal, closing connection", name)
				conn.Close()
				return
			case <-done:
				klog.Warningf("[%s] Message handler exited, closing connection", name)
				conn.Close()
				break heartbeatLoop
			case <-heartTicker.C:
				err := writeMsgToConn(conn, beat)
				if err != nil {
					heartbeatFailures++
					klog.Errorf("[%s] Heartbeat failed (%d/%d): %v", name, heartbeatFailures, maxHeartbeatFailures, err)
					if heartbeatFailures >= maxHeartbeatFailures {
						klog.Errorf("[%s] Too many heartbeat failures, reconnecting...", name)
						conn.Close()
						break heartbeatLoop
					}
				} else {
					heartbeatFailures = 0
				}
			}
		}
		heartTicker.Stop()
		// Loop will retry connection
	}
}

func Run(controllerManagerAddress string, fastFuncControllerAddress string, mps bool) {
	klog.Infof("Starting FaST-GShare configurator...")

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

	hostname, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Error Cannot get hostname. \n")
		panic(err)
	}

	helloMessage := types.ConfiguratorNodeHelloMessage{
		Hostname: hostname,
		GrpcPort: 5001,
	}

	// Start controller-manager connection goroutine
	go maintainConnection(
		"controller-manager",
		controllerManagerAddress,
		helloMessage,
		sigChan,
		recvMsgAndWriteConfig, // message handler for controller-manager
	)

	// Start fastfunc-controller connection goroutine (no message handler)
	go maintainConnection(
		"fastfunc-controller",
		fastFuncControllerAddress,
		helloMessage,
		sigChan,
		nil, // no message handler for now
	)

	// Wait for signal
	<-sigChan
	klog.Infof("Received signal, shutting down main process")
	server.Stop()
}

func writeMsgToConn(conn net.Conn, b []byte) error {
	_, err := conn.Write(b)
	if err != nil {
		klog.Errorf("Error failed to write msg: %v", err)
		return err
	}
	return nil
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

	for range heartTick {
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
