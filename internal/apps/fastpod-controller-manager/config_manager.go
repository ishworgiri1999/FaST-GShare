/*
Copyright 2024 FaST-GShare Authors, KontonGu (Jianfeng Gu), et. al.

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
	"bytes"
	"container/list"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
)

type NodeStatus string

const (
	NodeReady    NodeStatus = "Ready"
	NodeNotReady NodeStatus = "NotReady"
)

type NodeLiveness struct {
	ConfigConn    net.Conn
	Status        NodeStatus
	LastHeartbeat time.Time
}

var (
	nodesLiveness    map[string]*NodeLiveness
	nodesLivenessMtx sync.Mutex
	configNetAddr    string = "0.0.0.0:10087"
	checkTickerItv   int    = 240
	kubeClient       kubernetes.Interface
)

func init() {
	nodesLiveness = make(map[string]*NodeLiveness)
}

// listen and initialize the gpu information received from each node and periodically check liveness of node;
// send pods' resource configuration to the node's configurator
func (ctr *Controller) startConfigManager(stopCh <-chan struct{}, kube_client kubernetes.Interface) error {
	klog.Infof("Starting the configuration manager of the controller manager .... ")
	// listenr of the socket connection from the configurator of each node
	connListen, err := net.Listen("tcp", configNetAddr)
	if err != nil {
		klog.Errorf("Error while listening to the tcp socket %s from configurator, %s", configNetAddr, err.Error())
		return err
	}
	defer connListen.Close()

	// check liveness of the gpu work node periodically
	checkTicker := time.NewTicker(time.Second * time.Duration(checkTickerItv))
	go ctr.checkNodeLiveness(checkTicker.C)

	kubeClient = kube_client
	klog.Infof("Listening to the nodes' connection .... ")
	// accept each node connection

	go func() {
		<-stopCh
		for _, node := range nodesLiveness {
			if node != nil {
				node.ConfigConn.Close()
			}
		}
		connListen.Close()
		klog.Infof("Stop the configuration manager of the controller manager .... ")
	}()

	for {
		conn, err := connListen.Accept()
		if err != nil {
			klog.Errorf("Error while accepting the tcp socket: %s", err.Error())
			continue
		}
		klog.Infof("Received the connection from a node with IP:Port = %s", conn.RemoteAddr().String())
		go ctr.handleNodeConnection(conn)
	}

}

// check liveness of the gpu work nodes every `tick` seconds
func (ctr *Controller) checkNodeLiveness(tick <-chan time.Time) {
	for {
		<-tick
		nodesLivenessMtx.Lock()
		curTime := time.Now()
		// traverse all nodes for status and liveness check
		for _, val := range nodesLiveness {
			itv := curTime.Sub(val.LastHeartbeat).Seconds()
			if itv > float64(checkTickerItv) {
				val.Status = NodeNotReady
			}
		}
		nodesLivenessMtx.Unlock()
	}
}

// update the node annotation to include gpu uuid, type, mem and scheduler port information
// for each GPU devices in the node

// Deprecated: this function is not used anywhere
func (ctr *Controller) updateNodeGPUAnnotation(nodeName string, uuid2port, uuid2mem, uuid2type *map[string]string) {
	node, err := kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error when update node annotation of gpu information, nodeName=%s.", nodeName)
		return
	}
	dcNode := node.DeepCopy()
	if dcNode.ObjectMeta.Annotations == nil {
		dcNode.ObjectMeta.Annotations = make(map[string]string)
	}
	var buf bytes.Buffer
	klog.Infof("updateNodeGPUAnnotation start buf write.")
	for key, val := range *uuid2mem {
		buf.WriteString(key + "_" + (*uuid2type)[key])
		buf.WriteString(":")
		buf.WriteString(val + "_" + (*uuid2port)[key])
		buf.WriteString(",")
	}
	klog.Infof("updateNodeGPUAnnotation end buf write.")
	devsInfoMsg := buf.String()
	dcNode.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUsINfo] = devsInfoMsg
	klog.Infof("Update the node = %s 's gpu information annoation to be: %s.", nodeName, devsInfoMsg)
	_, err = kubeClient.CoreV1().Nodes().Update(context.TODO(), dcNode, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Error while Updating the node = %s 's gpu information annoation to be: %s.", nodeName, devsInfoMsg)
		return
	}

}

// update pods' gpu resource configuration to configurator
func (ctr *Controller) updatePodsGPUConfig(nodeName, uuid string, podlist *list.List) error {
	nodesLivenessMtx.Lock()
	nodeLive, has := nodesLiveness[nodeName]
	nodesLivenessMtx.Unlock()
	if !has {
		errMsg := fmt.Sprintf("The node = %s is not initialized.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}

	if nodeLive.Status != NodeReady {
		errMsg := fmt.Sprintf("The node = %s is not ready.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	// Write and Send podlist's GPU resource configuration to the configurator

	podGPUConfigRequest := seti.UpdateMPSConfigsRequest{
		DeviceUuid:        uuid,
		FastpodGpuConfigs: make([]*seti.FastPodGPUConfig, 0, podlist.Len()),
	}

	// write resource configuration
	if podlist != nil {
		for pod := podlist.Front(); pod != nil; pod = pod.Next() {
			podRequest := pod.Value.(*PodReq)
			podGPUConfigRequest.FastpodGpuConfigs = append(podGPUConfigRequest.FastpodGpuConfigs, &seti.FastPodGPUConfig{
				Key:         podRequest.Key,
				QtRequest:   podRequest.QtRequest,
				QtLimit:     podRequest.QtLimit,
				SmPartition: int64(podRequest.SMPartition),
				Memory:      podRequest.Memory,
			})
		}
	}
	node := nodes[nodeName]
	if node == nil {
		errMsg := fmt.Sprintf("The node = %s is not initialized.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	client := node.grpcClient
	if client == nil {
		errMsg := fmt.Sprintf("The node = %s is not initialized.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// send the request to the configurator
	_, err := client.UpdateMPSConfigs(ctx, &podGPUConfigRequest)
	if err != nil {
		klog.Errorf("Error while sending the pod gpu resource configuration to the configurator of node = %s, %s", nodeName, err.Error())
		return err
	}

	klog.Infof("Update the gpu device = %s in the node = %s resource configuration with: %v", uuid, nodeName, podGPUConfigRequest)

	return nil
}
