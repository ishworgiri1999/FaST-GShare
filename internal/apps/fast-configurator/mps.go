package fastconfigurator

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// MPSServer represents an NVIDIA Multi-Process Service server instance
type MPSServer struct {
	UUID      string // Device UUID
	Name      string // Device name
	isEnabled bool   // Track if MPS is currently enabled
}

// SetupMPSEnvironment sets up directories and MPS daemon for a specific UUID
func (m *MPSServer) SetupMPSEnvironment() error {
	if err := m.CreateDirectories(); err != nil {
		return fmt.Errorf("failed to create MPS directories: %w", err)
	}

	if err := m.StartMPSDaemon(); err != nil {
		// Attempt to clean up directories if daemon start fails
		_ = m.CleanupDirectories()
		return fmt.Errorf("failed to start MPS daemon: %w", err)
	}

	return nil
}

func (m *MPSServer) GetLogDir() string {
	return fmt.Sprintf("/tmp/mps_log_%s", m.UUID)
}
func (m *MPSServer) GetPipeDir() string {
	return fmt.Sprintf("/tmp/mps_%s", m.UUID)
}

// CreateDirectories creates required directories for MPS
func (m *MPSServer) CreateDirectories() error {
	dirs := []string{
		fmt.Sprintf("/tmp/mps_%s", m.UUID),
		fmt.Sprintf("/tmp/mps_log_%s", m.UUID),
	}

	for _, dir := range dirs {
		// Clean up existing directories first
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("Warning: failed to remove existing directory %s: %v", dir, err)
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// CleanupDirectories removes MPS directories
func (m *MPSServer) CleanupDirectories() error {
	dirs := []string{
		fmt.Sprintf("/tmp/mps_%s", m.UUID),
		fmt.Sprintf("/tmp/mps_log_%s", m.UUID),
	}

	var lastErr error
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			lastErr = fmt.Errorf("failed to remove directory %s: %w", dir, err)
			log.Printf("Warning: %v", lastErr)
		}
	}
	return lastErr
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
	if m.isEnabled {
		log.Printf("MPS daemon already started for GPU %s: %s", m.Name, m.UUID)
		return nil
	}

	env := m.BuildEnvironment()
	cmd := exec.Command("nvidia-cuda-mps-control", "-d")
	cmd.Env = append(os.Environ(), env...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start MPS daemon: %w", err)
	}

	// Give the daemon a moment to start and verify it's running
	time.Sleep(500 * time.Millisecond)
	if running, err := m.IsMPSDaemonRunning(); err != nil {
		log.Printf("Warning: couldn't verify MPS daemon status: %v", err)
	} else if !running {
		return fmt.Errorf("MPS daemon failed to start properly")
	}

	m.isEnabled = true
	log.Printf("MPS daemon started for GPU %s: %s with environment: %v", m.Name, m.UUID, env)
	return nil
}

// IsMPSDaemonRunning checks if the MPS daemon is currently running
func (m *MPSServer) IsMPSDaemonRunning() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ps", "-ef")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute ps command: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "nvidia-cuda-mps-control") &&
			strings.Contains(line, m.UUID) {
			return true, nil
		}
	}
	return false, nil
}

// StopMPSDaemon stops the MPS daemon for the specified GPU
func (m *MPSServer) StopMPSDaemon() error {
	if !m.isEnabled {
		return nil
	}

	env := m.BuildEnvironment()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-cuda-mps-control")
	cmd.Env = append(os.Environ(), env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start nvidia-cuda-mps-control: %w", err)
	}

	if _, err := stdin.Write([]byte("quit\n")); err != nil {
		return fmt.Errorf("failed to write quit command: %w", err)
	}

	if err := stdin.Close(); err != nil {
		log.Printf("Warning: error closing stdin pipe: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to stop MPS daemon: %w", err)
	}

	// Verify MPS daemon has stopped
	time.Sleep(500 * time.Millisecond)
	if running, err := m.IsMPSDaemonRunning(); err != nil {
		log.Printf("Warning: couldn't verify MPS daemon status: %v", err)
	} else if running {
		return fmt.Errorf("MPS daemon is still running after stop command")
	}

	// Clean up directories after successful stop
	if err := m.CleanupDirectories(); err != nil {
		log.Printf("Warning: failed to clean up MPS directories: %v", err)
	}

	m.isEnabled = false
	log.Printf("MPS daemon stopped for GPU %s: %s", m.Name, m.UUID)
	return nil
}

// ListMPSProcesses lists processes containing "mps"
func (m *MPSServer) ListMPSProcesses() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ps", "-ef")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %w", err)
	}

	var results []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "mps") {
			results = append(results, line)
		}
	}
	return results, nil
}

// NewMPSServer creates a new MPSServer instance
func NewMPSServer(name, uuid string) (*MPSServer, error) {
	if uuid == "" {
		return nil, fmt.Errorf("UUID cannot be empty")
	}

	if name == "" {
		name = "unnamed-gpu"
		log.Printf("Warning: Creating MPS server with empty name, using default: %s", name)
	}

	return &MPSServer{
		UUID:      uuid,
		Name:      name,
		isEnabled: false,
	}, nil
}
