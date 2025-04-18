package types

import (
	"encoding/json"
)

type ControllerMessage struct {
	UUID           string
	FastSchedConf  string
	GPUClientsPort string
}

type ConfiguratorHeartbeatMessage struct {
	Alive bool `json:"alive"`
}

type ConfiguratorNodeHelloMessage struct {
	Hostname string
	GrpcPort int
}

type ConfiguratorNodeAckMessage struct {
	Ok bool `json:"ok"`
}

type VirtualGPU struct {
	MemoryBytes         uint64
	MultiProcessorCount int
	IsProvisioned       bool
	InUse               bool
	ProvisionedGPU      *GPU
}

type GPU struct {
	UUID     string
	TypeName string
	Memory   uint64
}

type GPURegisterMessage struct {
	GPU []VirtualGPU
}

type PodGPURequest struct {
	// PodName is the name of the pod namespace+name
	Key           string
	QtRequest     float64
	QtLimit       float64
	SMPartition   int64
	Memory        int64
	GPUClientPort int
}
type UpdatePodsGPUConfigMessage struct {
	GpuUUID        string
	PodGPURequests []PodGPURequest
}

// EncodeToByte encodes a struct into a byte slice with a delimiter '\n' byte at the end
func EncodeToByte(data interface{}) ([]byte, error) {
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	// Add delimiter byte at the end
	buf = append(buf, '\n')
	return buf, nil
}

// DecodeFromByte decodes a byte slice into a struct (must end with '\n')
func DecodeFromByte(data []byte, v interface{}) error {
	// Remove delimiter byte before decoding
	if len(data) > 0 && (data[len(data)-1] == '\n') {
		data = data[:len(data)-1]
	}

	err := json.Unmarshal(data, v)
	return err
}
