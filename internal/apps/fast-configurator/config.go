package fastconfigurator

import (
	"fmt"
	"strings"
)

var SmMap = map[string]int{
	"t1000": 16,
	"a100":  108,
	"v100":  84,
}

func GetSMCount(gpuNameFull string) (int, error) {

	gpuName := strings.ToLower(gpuNameFull)

	if strings.Contains(gpuName, "a100") {
		gpuName = "a100"
	} else if strings.Contains(gpuName, "v100") {
		gpuName = "v100"
	} else if strings.Contains(gpuName, "t1000") {
		gpuName = "t1000"
	}

	// Use the nvidia-smi command to get the SM count
	smCount, exists := SmMap[gpuName]
	if !exists {
		return 0, fmt.Errorf("GPU not found: %s", gpuName)
	}

	return smCount, nil
}
