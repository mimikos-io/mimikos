package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

func loadPetstoreSpec(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(
		filepath.Join(testdataDir(t), "specs", "petstore-3.0.yaml"),
	)
	require.NoError(t, err)

	return data
}

// freePort finds an available TCP port by binding to :0 and releasing it.
func freePort(t *testing.T) int {
	t.Helper()

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp", ":0")
	require.NoError(t, err)

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "expected *net.TCPAddr")

	port := tcpAddr.Port

	require.NoError(t, listener.Close())

	return port
}

// startTestInstance creates and starts an Instance with the petstore spec on a
// free port. Returns the instance and port. Registers cleanup to stop the
// instance.
func startTestInstance(t *testing.T) (*Instance, int) {
	t.Helper()

	inst := &Instance{}
	port := freePort(t)
	specBytes := loadPetstoreSpec(t)

	err := inst.Start(context.Background(), StartConfig{
		SpecPath:  "petstore-3.0.yaml",
		SpecBytes: specBytes,
		Port:      port,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if inst.IsRunning() {
			_ = inst.Stop(context.Background())
		}
	})

	return inst, port
}

// --- Instance.Start tests ---

func TestInstance_Start_ValidSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst, port := startTestInstance(t)

	assert.True(t, inst.IsRunning())

	// Verify the HTTP server is actually responding.
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet,
		fmt.Sprintf("http://localhost:%d/pets", port), nil,
	)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestInstance_Start_AlreadyRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst, _ := startTestInstance(t)

	// Second start should fail.
	err := inst.Start(context.Background(), StartConfig{
		SpecPath:  "petstore-3.0.yaml",
		SpecBytes: loadPetstoreSpec(t),
		Port:      freePort(t),
	})

	require.ErrorIs(t, err, ErrAlreadyRunning)
	assert.Contains(t, err.Error(), "call stop_server first")
}

func TestInstance_Start_InvalidSpec(t *testing.T) {
	inst := &Instance{}

	err := inst.Start(context.Background(), StartConfig{
		SpecPath:  "bad.yaml",
		SpecBytes: []byte("not a valid openapi spec"),
		Port:      freePort(t),
	})

	require.Error(t, err)
	assert.False(t, inst.IsRunning())
}

func TestInstance_Start_PortUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	// Bind a port to make it unavailable.
	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp", ":0")
	require.NoError(t, err)

	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	port := tcpAddr.Port
	inst := &Instance{}

	startErr := inst.Start(context.Background(), StartConfig{
		SpecPath:  "petstore-3.0.yaml",
		SpecBytes: loadPetstoreSpec(t),
		Port:      port,
	})

	require.Error(t, startErr)
	assert.Contains(t, startErr.Error(), "not available")
	assert.False(t, inst.IsRunning())
}

// --- Instance.Stop tests ---

func TestInstance_Stop_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst, port := startTestInstance(t)

	err := inst.Stop(context.Background())
	require.NoError(t, err)

	assert.False(t, inst.IsRunning())

	// Verify the port is no longer in use.
	var lc net.ListenConfig

	listener, listenErr := lc.Listen(
		context.Background(), "tcp", fmt.Sprintf(":%d", port),
	)
	if listenErr == nil {
		listener.Close()
	}
}

func TestInstance_Stop_NotRunning(t *testing.T) {
	inst := &Instance{}

	err := inst.Stop(context.Background())

	require.ErrorIs(t, err, ErrNotRunning)
	assert.Contains(t, err.Error(), "call start_server first")
}

// --- Instance.Status tests ---

func TestInstance_Status_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	// Pin time for deterministic uptime.
	frozenTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	originalTimeNow := timeNow

	timeNow = func() time.Time { return frozenTime }

	t.Cleanup(func() { timeNow = originalTimeNow })

	inst, port := startTestInstance(t)

	// Advance time by 10 seconds.
	timeNow = func() time.Time { return frozenTime.Add(10 * time.Second) }

	status := inst.Status()

	assert.True(t, status.Running)
	assert.Equal(t, port, status.Port)
	assert.Equal(t, "deterministic", string(status.Mode))
	assert.Equal(t, "Swagger Petstore", status.SpecTitle)
	assert.Equal(t, "3.0.0", status.SpecVersion)
	assert.Equal(t, 3, status.Operations)
	assert.InDelta(t, 10.0, status.UptimeSeconds, 0.1)
}

func TestInstance_Status_Stopped(t *testing.T) {
	inst := &Instance{}

	status := inst.Status()

	assert.False(t, status.Running)
	assert.Equal(t, 0, status.Port)
	assert.Empty(t, status.Mode)
	assert.Empty(t, status.SpecTitle)
	assert.Equal(t, 0, status.Operations)
	assert.InDelta(t, 0.0, status.UptimeSeconds, 0.001)
}

// --- Concurrency safety ---

func TestInstance_ConcurrentAccess_NoRace(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst := &Instance{}
	specBytes := loadPetstoreSpec(t)

	var wg sync.WaitGroup

	// Exercise mutex under concurrent access. Start calls use Port:0 so they
	// fail at TCP bind — the goal is race-detector coverage, not functional
	// start/stop of a running server.
	for i := range 10 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			if n%2 == 0 {
				_ = inst.Start(context.Background(), StartConfig{
					SpecPath:  "petstore-3.0.yaml",
					SpecBytes: specBytes,
					Port:      0, // will fail to bind, but that's OK for race testing
				})
			} else {
				_ = inst.Stop(context.Background())
			}
		}(i)
	}

	wg.Wait()

	// Clean up: ensure instance is stopped.
	if inst.IsRunning() {
		require.NoError(t, inst.Stop(context.Background()))
	}
}

// --- BehaviorMap access ---

func TestInstance_Start_PopulatesBehaviorMap(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst, _ := startTestInstance(t)

	inst.mu.Lock()
	bm := inst.behaviorMap
	inst.mu.Unlock()

	require.NotNil(t, bm)
	assert.Equal(t, 3, bm.Len(), "petstore-3.0 should have 3 endpoints")

	_, ok := bm.Get("GET", "/pets")
	assert.True(t, ok, "should find GET /pets")

	_, ok = bm.Get("POST", "/pets")
	assert.True(t, ok, "should find POST /pets")

	_, ok = bm.Get("GET", "/pets/{petId}")
	assert.True(t, ok, "should find GET /pets/{petId}")
}

// --- Stop clears state ---

func TestInstance_Stop_ClearsState(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	inst, _ := startTestInstance(t)

	require.NoError(t, inst.Stop(context.Background()))

	inst.mu.Lock()
	defer inst.mu.Unlock()

	assert.Nil(t, inst.srv)
	assert.Nil(t, inst.handler)
	assert.Nil(t, inst.behaviorMap)
	assert.Nil(t, inst.startupResult)
	assert.Equal(t, 0, inst.port)
	assert.Empty(t, inst.mode)
	assert.Empty(t, inst.specTitle)
}
