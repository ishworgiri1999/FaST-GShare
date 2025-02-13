package fastconfigurator

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func StartMPS(gpus []*GPU) error {
	for _, gpu := range gpus {
		if err := SetupMPSEnvironment(gpu); err != nil {
			log.Printf("failed to set up MPS environment for %s: %v", gpu.UUID, err)
			return err
		}
	}
	return nil
}

func StopMPS(gpus []*GPU) {
	for _, gpu := range gpus {
		if err := StopMPSDaemon(gpu); err != nil {
			log.Printf("failed to stop MPS daemon for %s: %v", gpu.UUID, err)
		}
	}
}

// Set up directories and MPS daemon for a specific UUID
func SetupMPSEnvironment(gpu *GPU) error {
	if err := CreateDirectories(gpu.UUID); err != nil {
		return err
	}

	if err := StartMPSDaemon(gpu); err != nil {
		return err
	}

	return nil
}

// Create required directories for MPS
func CreateDirectories(uuid string) error {
	dirs := []string{
		fmt.Sprintf("/fastpod/mps/tmp/mps_%s", uuid),
		fmt.Sprintf("/fastpod/mps/tmp/mps_log_%s", uuid),
	}

	for _, dir := range dirs {
		os.RemoveAll(dir)
		if err := os.Mkdir(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Build environment variables for MPS
func BuildEnvironment(uuid string) []string {
	return []string{
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", uuid),
		fmt.Sprintf("CUDA_MPS_PIPE_DIRECTORY=/tmp/mps_%s", uuid),
		fmt.Sprintf("CUDA_MPS_LOG_DIRECTORY=/tmp/mps_log_%s", uuid),
	}
}

// Start MPS daemon with specified environment
func StartMPSDaemon(gpu *GPU) error {

	env := BuildEnvironment(gpu.UUID)
	cmd := exec.Command("nvidia-cuda-mps-control", "-d")
	cmd.Env = append(os.Environ(), env...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MPS daemon: %w", err)
	}

	log.Printf("MPS daemon started for gpu %s : %s with environment: %v", gpu.Name, gpu.UUID, env)
	return nil
}

func StopMPSDaemon(gpu *GPU) error {
	env := BuildEnvironment(gpu.UUID)

	cmd := exec.Command("nvidia-cuda-mps-control")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start nvidia-cuda-mps-control: %w", err)
	}

	if _, err := stdin.Write([]byte("quit\n")); err != nil {
		return fmt.Errorf("failed to write quit command: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to stop MPS daemon: %w", err)
	}
	log.Printf("MPS daemon stopped for gpu %s : %s", gpu.Name, gpu.UUID)

	return nil
}

// List processes containing "mps"
func listMPSProcesses() error {
	out, err := exec.Command("ps", "-ef").Output()
	if err != nil {
		return fmt.Errorf("ps command failed: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "mps") {
			fmt.Println(line)
		}
	}
	return nil
}
