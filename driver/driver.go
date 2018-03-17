package driver

import "errors"

// Record is a map of a single database record
type Record map[string]interface{}

// Args are used for populating a query
type Args map[string]interface{}

// Config is a map or configuration data specific to a specialized Driver
type Config map[string]interface{}

// Driver managed configuration and of a database and executes queries against it.
type Driver interface {
	Configure(config Config) error
	ConfigSurvey(config Config) error
	Execute(query string, args Args) (chan Record, error)
}

// Manager types
type Manager struct {
	// a map of of machine names to drivers
	drivers map[string]func() Driver
}

// AddDriver adds a driver to the DriverManager
func (m *Manager) AddDriver(machineName string, driverFactory func() Driver) {
	m.drivers[machineName] = driverFactory
}

// RegisteredDrivers returns a string slice of driver machine names
func (m *Manager) RegisteredDrivers() []string {
	drivers := make([]string, 0)

	for k := range m.drivers {
		drivers = append(drivers, k)
	}

	return drivers
}

// GetNewDriver returns a new un-configured Driver
func (m *Manager) GetNewDriver(machineName string) (Driver, error) {
	if driverFactory, ok := m.drivers[machineName]; ok {
		return driverFactory(), nil
	}

	return nil, errors.New("No such driver: " + machineName)
}

// NewManager creates a new driver manager
func NewManager() *Manager {
	return &Manager{
		drivers: make(map[string]func() Driver),
	}
}

// where drivers register
var DriverManager = NewManager()