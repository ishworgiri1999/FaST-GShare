package fastconfigurator

import "fmt"

var SmMap = map[string]int{
	"NVIDIA T1000":          16,
	"NVIDIA A100-PCIE-40GB": 108,
}

func GetSMCount(gpuName string) (int, error) {
	// Use the nvidia-smi command to get the SM count
	smCount, exists := SmMap[gpuName]
	if !exists {
		return 0, fmt.Errorf("GPU not found: %s", gpuName)
	}

	return smCount, nil
}
