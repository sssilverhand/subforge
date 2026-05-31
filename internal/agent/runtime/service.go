package runtime

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ServiceStatus represents the state of a systemd service.
type ServiceStatus struct {
	Name    string
	Active  bool
	Running bool
	Since   *time.Time
}

// ServiceManager wraps systemctl for managing xray and hysteria2.
type ServiceManager struct{}

func NewServiceManager() *ServiceManager { return &ServiceManager{} }

// Status returns current status of a systemd service.
func (m *ServiceManager) Status(name string) (*ServiceStatus, error) {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	active := strings.TrimSpace(string(out)) == "active"
	// is-active returns exit code 3 if not active — not a real error
	_ = err

	s := &ServiceStatus{
		Name:    name,
		Active:  active,
		Running: active,
	}
	return s, nil
}

// Start starts a systemd service.
func (m *ServiceManager) Start(name string) error {
	return runSystemctl("start", name)
}

// Stop stops a systemd service.
func (m *ServiceManager) Stop(name string) error {
	return runSystemctl("stop", name)
}

// Restart restarts a systemd service.
func (m *ServiceManager) Restart(name string) error {
	return runSystemctl("restart", name)
}

// Enable enables a service to start on boot.
func (m *ServiceManager) Enable(name string) error {
	return runSystemctl("enable", name)
}

// Reload sends SIGHUP to reload config without full restart.
func (m *ServiceManager) Reload(name string) error {
	return runSystemctl("reload", name)
}

// ReloadOrRestart reloads if supported, falls back to restart.
func (m *ServiceManager) ReloadOrRestart(name string) error {
	if err := runSystemctl("reload", name); err != nil {
		return runSystemctl("restart", name)
	}
	return nil
}

// DaemonReload runs systemctl daemon-reload (needed after unit file changes).
func (m *ServiceManager) DaemonReload() error {
	return runSystemctl("daemon-reload")
}

func runSystemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}
