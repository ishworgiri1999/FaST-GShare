package fastpodcontrollermanager

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"

	"github.com/KontonGu/FaST-GShare/pkg/grpcclient"
	"github.com/KontonGu/FaST-GShare/pkg/libs/bitmap"
	"github.com/KontonGu/FaST-GShare/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// intialize GPU device information in nodesInfo and send resource configuration of podList within a GPU
// device to nodes' configurator
func (ctr *Controller) handleNodeConnection(conn net.Conn) error {
	nodeIP := strings.Split(conn.RemoteAddr().String(), ":")[0]
	reader := bufio.NewReader(conn)

	var err error
	// get hostname of the node, hostname here is the daemonset's pod name.
	res, err := reader.ReadBytes('\n')
	if err != nil {
		klog.Errorf("Error while reading hostname from node information.")
		klog.Errorf("Error: %v", err)
		return err
	}
	var helloMessage types.ConfiguratorNodeHelloMessage
	err = types.DecodeFromByte(res, &helloMessage)
	if err != nil {
		klog.Errorf("Error while parsing hostname from node information.")
		klog.Errorf("Error: %v", err)
		return err
	}
	nodeHostName := helloMessage.Hostname

	klog.Infof("Received hostname from node information: %s", helloMessage.Hostname)
	daemonPodName := nodeHostName
	daemonPod, err := kubeClient.CoreV1().Pods("kube-system").Get(context.TODO(), daemonPodName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error cannot find the node daemonset. Using the default node name.")
	}
	if len(daemonPod.Spec.NodeName) > 0 {

		nodeHostName = daemonPod.Spec.NodeName
		klog.Infof("Node name is: %s", nodeHostName)
	}

	hostName := nodeHostName
	//ack the hello message
	ackMsg := types.ConfiguratorNodeAckMessage{
		Ok: true,
	}
	ackMsgBytes, err := types.EncodeToByte(ackMsg)

	if err != nil {
		klog.Errorf("Error while encoding the ack message.")
		klog.Errorf("Error: %v", err)
		return err
	}

	_, err = conn.Write(ackMsgBytes)
	if err != nil {
		klog.Errorf("Error while sending the ack message.")
		klog.Errorf("Error: %v", err)
		return err
	}

	klog.Infof("Received hostname from node information: %s", helloMessage.Hostname)

	client := grpcclient.NewGrpcClient()

	err = client.Connect(nodeIP, helloMessage.GrpcPort)
	if err != nil {
		klog.Errorf("Error while connecting to the node configurator.")
		klog.Errorf("Error: %v", err)
	}

	health, err := client.Health(context.TODO())
	if err != nil {
		klog.Errorf("Error while checking the health of the node configurator.")
		klog.Errorf("Error: %v", err)
		return err
	}

	response, err := client.GetAvailableGPUs(context.TODO())
	if err != nil {
		klog.Errorf("Error while getting the available GPUs from the node configurator.")
	}

	nodesInfoMtx.Lock()
	if nodes[hostName] != nil {
		klog.Info(nodes[hostName].vGPUID2GPU)
	}
	// after succful conn. save the connection to the node configurator

	if node, has := nodes[hostName]; !has {
		node = &Node{
			vGPUID2GPU:      make(map[string]*GPUDevInfo),
			UUID2SchedPort:  make(map[string]string),
			UUID2GPUType:    make(map[string]string),
			DaemonIP:        nodeIP,
			DaemonPortAlloc: bitmap.NewBitmap(PortRange),
			vgpus:           response.Gpus,
			hostName:        hostName,
			grpcClient:      client,
		}
		nodes[hostName] = node
		klog.Infof("gpu inside node len %d", len(node.vgpus))
	} else {
		node.vgpus = response.Gpus
		node.grpcClient = client
		klog.Infof("gpu inside node len %d", len(node.vgpus))
		node.DaemonIP = nodeIP
	}

	nodesInfoMtx.Unlock()

	klog.Infof("Received available GPUs from node configurator: %v", hostName)

	defer client.Close()

	klog.Infof("Received health check response from node configurator: %v", health)

	// nodeName := hostName

	// var gpuInfoMsg types.GPURegisterMessage

	// // get gpu device information
	// d, err := reader.ReadBytes('\n')
	// if err != nil {
	// 	klog.Error("Error while reading gpu device information.")
	// 	return
	// }
	// err = types.DecodeFromByte(d, &gpuInfoMsg)
	// if err != nil {
	// 	klog.Error("Error while parsing gpu device information.")
	// 	return
	// }
	// klog.Infof("Received gpu device information")
	// for _, gpu := range gpuInfoMsg.GPU {
	// 	klog.Infof("GPU: %v", gpu)
	// }

	// devsNum := len(gpuInfoMsg.GPU)
	// klog.Infof("GPU Device number is %d.", devsNum)
	// // scheduler port for each node starts from 52001
	// schedPort := GPUSchedPortStart
	// uuid2port := make(map[string]string, devsNum)
	// uuid2mem := make(map[string]uint64, devsNum)
	// uuid2type := make(map[string]string, devsNum)
	// var uuidList []string
	// for i := 0; i < devsNum; i++ {

	// 	// infoSplit := strings.Split(devsInfo[i], ":")
	// 	// uuidType, mem := infoSplit[0], infoSplit[1]
	// 	// uuidTypeSplit := strings.Split(uuidType, "_")
	// 	// uuid, devType := uuidTypeSplit[0], uuidTypeSplit[1]
	// 	uuid := gpuInfoMsg.GPU[i].UUID
	// 	uuid2port[uuid] = strconv.Itoa(schedPort)
	// 	schedPort += 1
	// 	uuid2mem[uuid] = gpuInfoMsg.GPU[i].Memory
	// 	uuid2type[uuid] = gpuInfoMsg.GPU[i].TypeName
	// 	uuidList = append(uuidList, uuid)
	// 	klog.Infof("Device Info: uuid = %s, type = %s, mem = %d.", uuid, uuid2type[uuid], uuid2mem[uuid])
	// }

	// // update nodesInfo and create dummyPod
	// nodesInfoMtx.Lock()

	// if nodesInfo[nodeName] != nil {
	// 	klog.Info(nodesInfo[nodeName].vGPUID2GPU)
	// }

	// if node, has := nodesInfo[nodeName]; !has {
	// 	pBm := bitmap.NewBitmap(PortRange)
	// 	pBm.Set(0)
	// 	node = &NodeStatusInfo{
	// 		vGPUID2GPU:      make(map[string]*GPUDevInfo),
	// 		UUID2SchedPort:  uuid2port,
	// 		UUID2GPUType:    uuid2type,
	// 		DaemonIP:        nodeIP,
	// 		DaemonPortAlloc: pBm,
	// 	}
	// 	for _, uuid := range uuidList {
	// 		vgpuID := fastpodv1.GenerateGPUID(8)
	// 		mem := uuid2mem[uuid]
	// 		node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
	// 			GPUType: uuid2type[uuid],
	// 			UUID:    uuid,
	// 			//TODO: change Mem to also uint64
	// 			Mem:      int64(mem),
	// 			Usage:    0.0,
	// 			UsageMem: 0,
	// 			PodList:  list.New(),
	// 		}
	// 		nodesInfo[nodeName] = node
	// 		go ctr.createDummyPod(nodeName, vgpuID, uuid2type[uuid], uuid)
	// 	}
	// } else {
	// 	// For the case of controller-manager crashed and recovery;
	// 	node.UUID2SchedPort = uuid2port
	// 	node.DaemonIP = nodeIP
	// 	node.UUID2GPUType = uuid2type
	// 	usedUuid := make(map[string]string)
	// 	for key, gpuinfo := range nodesInfo[nodeName].vGPUID2GPU {
	// 		usedUuid[gpuinfo.UUID] = key
	// 	}
	// 	for _, uuid := range uuidList {
	// 		if key, hastmp := usedUuid[uuid]; !hastmp {
	// 			vgpuID := fastpodv1.GenerateGPUID(8)
	// 			mem, _ := uuid2mem[uuid]
	// 			node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
	// 				GPUType: uuid2type[uuid],
	// 				UUID:    uuid,
	// 				//TODO: change Mem to also uint64
	// 				Mem:      int64(mem),
	// 				Usage:    0.0,
	// 				UsageMem: 0,
	// 				PodList:  list.New(),
	// 			}
	// 			go ctr.createDummyPod(nodeName, vgpuID, uuid2type[uuid], uuid)
	// 		} else {
	// 			//update memory
	// 			node.vGPUID2GPU[key].Mem = int64(uuid2mem[uuid])

	// 		}
	// 	}
	// }
	// nodesInfoMtx.Unlock()

	nodeName := hostName
	// Initialize node liveness
	nodesLivenessMtx.Lock()
	if _, has := nodesLiveness[nodeName]; !has {
		nodesLiveness[nodeName] = &NodeLiveness{
			ConfigConn:    conn,
			LastHeartbeat: time.Time{},
			Status:        NodeReady,
		}
	} else {
		nodesLiveness[nodeName].ConfigConn = conn
		nodesLiveness[nodeName].LastHeartbeat = time.Time{}
		nodesLiveness[nodeName].Status = NodeReady
	}
	nodesLivenessMtx.Unlock()

	// uuidTomMemStringified := make(map[string]string)
	// for uuid, mem := range uuid2mem {
	// 	uuidTomMemStringified[uuid] = strconv.FormatUint(mem, 10)
	// }
	// update node annotation to include gpu device information
	// ctr.updateNodeGPUAnnotation(nodeName, &uuid2port, &uuidTomMemStringified, &uuid2type)
	// klog.Infof("updateNodeGPUAnnotation finished. nodeName=%s", nodeName)

	// update pods' gpu resource configuration to configurator
	// nodesInfoMtx.Lock()
	// for vgpuID, gpuDevInfo := range nodesInfo[nodeName].vGPUID2GPU {
	// 	klog.Infof("updatePodsGPUConfig started. vgpu_id = %s.", vgpuID)
	// 	ctr.updatePodsGPUConfig(nodeName, gpuDevInfo.UUID, gpuDevInfo.PodList)
	// }
	// nodesInfoMtx.Unlock()

	// check the hearbeats and update the ready status of the node

	for {
		heartbeatMsg, err := reader.ReadBytes('\n')
		hasError := false

		if err != nil {
			klog.Errorf("Error while reading heartbeat message from node %s.", nodeName)
			klog.Errorf("Error: %v", err)
			hasError = true
		}
		var heartBeat types.ConfiguratorHeartbeatMessage
		err = types.DecodeFromByte(heartbeatMsg, &heartBeat)

		if err != nil {
			klog.Errorf("Error while decoding heartbeat message from node %s.", nodeName)
			klog.Errorf("Error decoding: %v", err)
			hasError = true

		}

		if !heartBeat.Alive {
			klog.Errorf("Node %s is not alive.", nodeName)
			hasError = true
		}

		nodesLivenessMtx.Lock()
		if hasError {
			nodesLiveness[nodeName].Status = NodeNotReady
			nodesLivenessMtx.Unlock()
			return nil
		} else {
			nodesLiveness[nodeName].Status = NodeReady
			nodesLiveness[nodeName].LastHeartbeat = time.Now()
			klog.Infof("Received heartbeat from the node %s.", nodeName)
			nodesLivenessMtx.Unlock()
		}
	}
}
