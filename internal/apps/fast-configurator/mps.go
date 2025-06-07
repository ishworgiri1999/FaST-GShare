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
	if m.isEnabled {
		log.Printf("MPS daemon already started for GPU %s: %s", m.Name, m.UUID)
		return nil
	}

	// check if the mps daemon is running for the given UUID
	running, err := m.IsMPSDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check if MPS daemon is running: %w", err)
	}
	if running {
		log.Printf("MPS daemon already running for GPU %s: %s", m.Name, m.UUID)
		return nil
	}

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
		m.GetPipeDir(),
		m.GetLogDir(),
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
		m.GetPipeDir(),
		m.GetLogDir(),
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
		fmt.Sprintf("CUDA_MPS_PIPE_DIRECTORY=%s", m.GetPipeDir()),
		fmt.Sprintf("CUDA_MPS_LOG_DIRECTORY=%s", m.GetLogDir()),
	}
}

// StartMPSDaemon starts the MPS daemon with -d (daemonize).
func (m *MPSServer) StartMPSDaemon() error {
	if m.isEnabled {
		log.Printf("MPS daemon already started for GPU %s: %s", m.Name, m.UUID)
		return nil
	}

	env := m.BuildEnvironment()
	// IMPORTANT: add "-d" to make it a true daemon
	cmd := exec.Command("nvidia-cuda-mps-control", "-d")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Use Run(), which does Start()+Wait() internally
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start MPS daemon: %w", err)
	}

	// Give it a moment
	time.Sleep(500 * time.Millisecond)

	// running, err := m.IsMPSDaemonRunning()
	// if err != nil {
	// 	log.Printf("Warning: couldn't verify MPS daemon status: %v", err)
	// } else if !running {
	// 	return fmt.Errorf("MPS daemon failed to start properly")
	// }

	m.isEnabled = true
	log.Printf("MPS daemon started for GPU %s (%s)",
		m.Name, m.UUID)

	return nil
}

// IsMPSDaemonRunning checks for the GPU‐specific MPS pipes.
// Returns true only if both "control" and "request" named pipes exist.
func (m *MPSServer) IsMPSDaemonRunning() (bool, error) {
	pipeDir := m.GetPipeDir() // e.g. "/tmp/mps_<UUID>"
	controlPipe := fmt.Sprintf("%s/control", pipeDir)
	fi, err := os.Stat(controlPipe)
	if err != nil {
		if os.IsNotExist(err) {
			// pipe isn’t there → daemon not running for this GPU
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", controlPipe, err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		log.Printf("Warning: %s exists but isn’t a pipe", controlPipe)
		// exists but isn’t a pipe
		return false, nil
	}

	return true, nil
}

// StopMPSDaemon cleanly tells MPS to quit, then removes dirs.
func (m *MPSServer) StopMPSDaemon() error {
	if !m.isEnabled {
		log.Printf("MPS daemon not running for GPU %s: %s", m.Name, m.UUID)
		return nil
	}

	// Use bash -c to pipe "quit" into the control binary
	cmd := exec.Command("bash", "-c", "echo quit | nvidia-cuda-mps-control")
	cmd.Env = append(os.Environ(), m.BuildEnvironment()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop MPS daemon: %w", err)
	}

	// Give it a moment to die
	time.Sleep(500 * time.Millisecond)
	if running, err := m.IsMPSDaemonRunning(); err != nil {
		log.Printf("Warning: couldn't verify MPS daemon status: %v", err)
	} else if running {
		return fmt.Errorf("MPS daemon is still running after quit")
	}

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
