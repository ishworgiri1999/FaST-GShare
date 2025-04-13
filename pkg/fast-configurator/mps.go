package fastconfigurator

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type MPSServer struct {
	UUID      string //device uuid
	Name      string //device name
	isEnabled bool
}

// SetupMPSEnvironment sets up directories and MPS daemon for a specific UUID
func (m *MPSServer) SetupMPSEnvironment() error {
	if err := m.CreateDirectories(); err != nil {
		return err
	}

	if err := m.StartMPSDaemon(); err != nil {
		return err
	}

	return nil
}

// CreateDirectories creates required directories for MPS
func (m *MPSServer) CreateDirectories() error {
	dirs := []string{
		fmt.Sprintf("/tmp/mps_%s", m.UUID),
		fmt.Sprintf("/tmp/mps_log_%s", m.UUID),
	}

	for _, dir := range dirs {
		os.RemoveAll(dir)
		if err := os.Mkdir(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// BuildEnvironment builds environment variables for MPS
func (m *MPSServer) BuildEnvironment() []string {
	return []string{
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", m.UUID),
		fmt.Sprintf("CUDA_MPS_PIPE_DIRECTORY=/tmp/mps_%s", m.UUID),
		fmt.Sprintf("CUDA_MPS_LOG_DIRECTORY=/tmp/mps_log_%s", m.UUID),
	}
}

// StartMPSDaemon starts MPS daemon with specified environment
func (m *MPSServer) StartMPSDaemon() error {
	env := m.BuildEnvironment()
	cmd := exec.Command("nvidia-cuda-mps-control", "-d")
	cmd.Env = append(os.Environ(), env...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MPS daemon: %w", err)
	}
	m.isEnabled = true
	log.Printf("MPS daemon started for gpu %s : %s with environment: %v", m.Name, m.UUID, env)
	return nil
}

// StopMPSDaemon stops the MPS daemon for the specified GPU
func (m *MPSServer) StopMPSDaemon(gpu *GPU) error {
	if gpu == nil {
		return fmt.Errorf("gpu is nil")
	}

	env := m.BuildEnvironment()

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

// ListMPSProcesses lists processes containing "mps"
func (m *MPSServer) ListMPSProcesses() error {
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

func NewMPSServer(name, uuid string) (*MPSServer, error) {

	return &MPSServer{
		UUID: uuid,
		Name: name,
	}, nil
}
