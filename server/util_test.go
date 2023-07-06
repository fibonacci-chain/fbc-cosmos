package server_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
)

var cancelledInPreRun = errors.New("Cancelled in prerun")

// Used in each test to run the function under test via Cobra
// but to always halt the command
func preRunETestImpl(cmd *cobra.Command, args []string) error {
	err := server.InterceptConfigsPreRunHandler(cmd, "", nil)
	if err != nil {
		return err
	}

	return cancelledInPreRun
}

func TestInterceptConfigsPreRunHandlerCreatesConfigFilesWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	cmd := server.StartCmd(nil, "/foobar", []trace.TracerProviderOption{})
	if err := cmd.Flags().Set(flags.FlagHome, tempDir); err != nil {
		t.Fatalf("Could not set home flag [%T] %v", err, err)
	}

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)
	if err := cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	// Test that config.toml is created
	configTomlPath := path.Join(tempDir, "config", "config.toml")
	s, err := os.Stat(configTomlPath)
	if err != nil {
		t.Fatalf("Could not stat config.toml after run %v", err)
	}

	if !s.Mode().IsRegular() {
		t.Fatal("config.toml not created as regular file")
	}

	if s.Size() == 0 {
		t.Fatal("config.toml created as empty file")
	}

	// Test that tendermint config is initialized
	if serverCtx.Config == nil {
		t.Fatal("tendermint config not created")
	}

	// Test that app.toml is created
	appTomlPath := path.Join(tempDir, "config", "app.toml")
	s, err = os.Stat(appTomlPath)
	if err != nil {
		t.Fatalf("Could not stat app.toml after run %v", err)
	}

	if !s.Mode().IsRegular() {
		t.Fatal("appp.toml not created as regular file")
	}

	if s.Size() == 0 {
		t.Fatal("config.toml created as empty file")
	}

	// Test that the config for use in server/start.go is created
	if serverCtx.Viper == nil {
		t.Error("app config Viper instance not created")
	}
}

func TestInterceptConfigsPreRunHandlerReadsConfigToml(t *testing.T) {
	const testDbBackend = "awesome_test_db"
	tempDir := t.TempDir()
	err := os.Mkdir(path.Join(tempDir, "config"), os.ModePerm)
	if err != nil {
		t.Fatalf("creating config dir failed: %v", err)
	}
	configTomlPath := path.Join(tempDir, "config", "config.toml")
	writer, err := os.Create(configTomlPath)
	if err != nil {
		t.Fatalf("creating config.toml file failed: %v", err)
	}

	_, err = writer.WriteString(fmt.Sprintf("db-backend = '%s'\n", testDbBackend))
	if err != nil {
		t.Fatalf("Failed writing string to config.toml: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed closing config.toml: %v", err)
	}

	cmd := server.StartCmd(nil, "/foobar", []trace.TracerProviderOption{})
	if err := cmd.Flags().Set(flags.FlagHome, tempDir); err != nil {
		t.Fatalf("Could not set home flag [%T] %v", err, err)
	}

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if testDbBackend != serverCtx.Config.DBBackend {
		t.Error("DBPath was not set from config.toml")
	}
}

func TestInterceptConfigsPreRunHandlerReadsAppToml(t *testing.T) {
	const testHaltTime = 1337
	tempDir := t.TempDir()
	err := os.Mkdir(path.Join(tempDir, "config"), os.ModePerm)
	if err != nil {
		t.Fatalf("creating config dir failed: %v", err)
	}
	appTomlPath := path.Join(tempDir, "config", "app.toml")
	writer, err := os.Create(appTomlPath)
	if err != nil {
		t.Fatalf("creating app.toml file failed: %v", err)
	}

	_, err = writer.WriteString(fmt.Sprintf("halt-time = %d\n", testHaltTime))
	if err != nil {
		t.Fatalf("Failed writing string to app.toml: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed closing app.toml: %v", err)
	}
	cmd := server.StartCmd(nil, tempDir, []trace.TracerProviderOption{})

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if testHaltTime != serverCtx.Viper.GetInt("halt-time") {
		t.Error("Halt time was not set from app.toml")
	}
}

func TestInterceptConfigsPreRunHandlerReadsFlags(t *testing.T) {
	const testAddr = "tcp://127.1.2.3:12345"
	tempDir := t.TempDir()
	cmd := server.StartCmd(nil, "/foobar", []trace.TracerProviderOption{})

	if err := cmd.Flags().Set(flags.FlagHome, tempDir); err != nil {
		t.Fatalf("Could not set home flag [%T] %v", err, err)
	}

	// This flag is added by tendermint
	if err := cmd.Flags().Set("rpc.laddr", testAddr); err != nil {
		t.Fatalf("Could not set address flag [%T] %v", err, err)
	}

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if testAddr != serverCtx.Config.RPC.ListenAddress {
		t.Error("RPCListenAddress was not set from command flags")
	}
}

func TestInterceptConfigsPreRunHandlerReadsEnvVars(t *testing.T) {
	const testAddr = "tcp://127.1.2.3:12345"
	tempDir := t.TempDir()
	cmd := server.StartCmd(nil, "/foobar", []trace.TracerProviderOption{})
	if err := cmd.Flags().Set(flags.FlagHome, tempDir); err != nil {
		t.Fatalf("Could not set home flag [%T] %v", err, err)
	}

	executableName, err := os.Executable()
	if err != nil {
		t.Fatalf("Could not get executable name: %v", err)
	}
	basename := path.Base(executableName)
	basename = strings.ReplaceAll(basename, ".", "_")
	// This is added by tendermint
	envVarName := fmt.Sprintf("%s_RPC_LADDR", strings.ToUpper(basename))
	os.Setenv(envVarName, testAddr)
	t.Cleanup(func() {
		os.Unsetenv(envVarName)
	})

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if testAddr != serverCtx.Config.RPC.ListenAddress {
		t.Errorf("RPCListenAddress was not set from env. var. %q", envVarName)
	}
}

/*
 The following tests are here to check the precedence of each
 of the configuration sources. A common setup functionality is used
 to avoid duplication of code between tests.
*/

var (
	TestAddrExpected    = "tcp://127.126.125.124:12345" // expected to be used in test
	TestAddrNotExpected = "tcp://127.127.127.127:11111" // not expected to be used in test
)

type precedenceCommon struct {
	envVarName     string
	flagName       string
	configTomlPath string

	cmd *cobra.Command
}

func newPrecedenceCommon(t *testing.T) precedenceCommon {
	retval := precedenceCommon{}

	// Determine the env. var. name based off the executable name
	executableName, err := os.Executable()
	if err != nil {
		t.Fatalf("Could not get executable name: %v", err)
	}
	basename := path.Base(executableName)
	basename = strings.ReplaceAll(basename, ".", "_")
	basename = strings.ReplaceAll(basename, "-", "_")
	// Store the name of the env. var.
	retval.envVarName = fmt.Sprintf("%s_RPC_LADDR", strings.ToUpper(basename))

	// Store the flag name. This flag is added by tendermint
	retval.flagName = "rpc.laddr"

	// Create a tempdir and create './config' under that
	tempDir := t.TempDir()
	err = os.Mkdir(path.Join(tempDir, "config"), os.ModePerm)
	if err != nil {
		t.Fatalf("creating config dir failed: %v", err)
	}
	// Store the path for config.toml
	retval.configTomlPath = path.Join(tempDir, "config", "config.toml")

	// always remove the env. var. after each test execution
	t.Cleanup(func() {
		// This should not fail but if it does just panic
		if err := os.Unsetenv(retval.envVarName); err != nil {
			panic("Could not clear configuration env. var. used in test")
		}
	})

	// Set up the command object that is used in this test
	retval.cmd = server.StartCmd(nil, tempDir, []trace.TracerProviderOption{})
	retval.cmd.PreRunE = preRunETestImpl

	return retval
}

func (v precedenceCommon) setAll(t *testing.T, setFlag *string, setEnvVar *string, setConfigFile *string) {
	if setFlag != nil {
		if err := v.cmd.Flags().Set(v.flagName, *setFlag); err != nil {
			t.Fatalf("Failed setting flag %q", v.flagName)
		}
	}

	if setEnvVar != nil {
		os.Setenv(v.envVarName, *setEnvVar)
	}

	if setConfigFile != nil {
		writer, err := os.Create(v.configTomlPath)
		if err != nil {
			t.Fatalf("creating config.toml file failed: %v", err)
		}

		_, err = writer.WriteString(fmt.Sprintf("[rpc]\nladdr = \"%s\"\n", *setConfigFile))
		if err != nil {
			t.Fatalf("Failed writing string to config.toml: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed closing config.toml: %v", err)
		}
	}
}

func TestInterceptConfigsPreRunHandlerPrecedenceFlag(t *testing.T) {
	testCommon := newPrecedenceCommon(t)
	testCommon.setAll(t, &TestAddrExpected, &TestAddrNotExpected, &TestAddrNotExpected)

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := testCommon.cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if TestAddrExpected != serverCtx.Config.RPC.ListenAddress {
		t.Fatalf("RPCListenAddress was not set from flag %q", testCommon.flagName)
	}
}

func TestInterceptConfigsPreRunHandlerPrecedenceEnvVar(t *testing.T) {
	testCommon := newPrecedenceCommon(t)
	testCommon.setAll(t, nil, &TestAddrExpected, &TestAddrNotExpected)

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := testCommon.cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if TestAddrExpected != serverCtx.Config.RPC.ListenAddress {
		t.Errorf("RPCListenAddress was not set from env. var. %q", testCommon.envVarName)
	}
}

func TestInterceptConfigsPreRunHandlerPrecedenceConfigFile(t *testing.T) {
	testCommon := newPrecedenceCommon(t)
	testCommon.setAll(t, nil, nil, &TestAddrExpected)

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := testCommon.cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if TestAddrExpected != serverCtx.Config.RPC.ListenAddress {
		t.Errorf("RPCListenAddress was not read from file %q", testCommon.configTomlPath)
	}
}

func TestInterceptConfigsPreRunHandlerPrecedenceConfigDefault(t *testing.T) {
	testCommon := newPrecedenceCommon(t)
	// Do not set anything

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)

	if err := testCommon.cmd.ExecuteContext(ctx); err != cancelledInPreRun {
		t.Fatalf("function failed with [%T] %v", err, err)
	}

	if "tcp://127.0.0.1:26657" != serverCtx.Config.RPC.ListenAddress {
		t.Error("RPCListenAddress is not using default")
	}
}

// Ensure that if interceptConfigs encounters any error other than non-existen errors
// that we correctly return the offending error, for example a permission error.
// See https://github.com/cosmos/cosmos-sdk/issues/7578
func TestInterceptConfigsWithBadPermissions(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "nonPerms")
	if err := os.Mkdir(subDir, 0600); err != nil {
		t.Fatalf("Failed to create sub directory: %v", err)
	}
	cmd := server.StartCmd(nil, "/foobar", []trace.TracerProviderOption{})
	if err := cmd.Flags().Set(flags.FlagHome, subDir); err != nil {
		t.Fatalf("Could not set home flag [%T] %v", err, err)
	}

	cmd.PreRunE = preRunETestImpl

	serverCtx := &server.Context{}
	ctx := context.WithValue(context.Background(), server.ServerContextKey, serverCtx)
	if err := cmd.ExecuteContext(ctx); !os.IsPermission(err) {
		t.Fatalf("Failed to catch permissions error, got: [%T] %v", err, err)
	}
}

func TestWaitForQuitSignals(t *testing.T) {
	t.Run("WithRestartChannelAndCanRestartAfterNotReached", func(t *testing.T) {
		restartCh := make(chan struct{})
		go func() {
			time.Sleep(100 * time.Millisecond)
			restartCh <- struct{}{}
		}()

		go func() {
			time.Sleep(200 * time.Millisecond)
			syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		}()

		errCode := server.WaitForQuitSignals(
			&server.Context{Logger: log.NewNopLogger()},
			restartCh,
			time.Now().Add(500*time.Millisecond),
		)
		expectedCode := int(syscall.SIGTERM) + 128
		if errCode.Code != expectedCode {
			t.Errorf("Expected error code %d, got %d", expectedCode, errCode.Code)
		}
	})

	t.Run("WithRestartChannelAndCanRestartAfterReached", func(t *testing.T) {
		restartCh := make(chan struct{})
		go func() {
			time.Sleep(100 * time.Millisecond)
			restartCh <- struct{}{}
		}()

		errCode := server.WaitForQuitSignals(
			&server.Context{Logger: log.NewNopLogger()},
			restartCh,
			time.Now().Add(-100*time.Millisecond),
		)
		if errCode.Code != server.RestartErrorCode {
			t.Errorf("Expected error code %d, got %d", server.RestartErrorCode, errCode.Code)
		}
	})

	t.Run("WithSIGINT", func(t *testing.T) {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT)

		go func() {
			time.Sleep(100 * time.Millisecond)
			syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		}()

		errCode := server.WaitForQuitSignals(
			&server.Context{Logger: log.NewNopLogger()},
			make(chan struct{}),
			time.Now(),
		)
		expectedCode := int(syscall.SIGINT) + 128
		if errCode.Code != expectedCode {
			t.Errorf("Expected error code %d, got %d", expectedCode, errCode.Code)
		}
	})

	t.Run("WithSIGTERM", func(t *testing.T) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		}()

		errCode := server.WaitForQuitSignals(
			&server.Context{Logger: log.NewNopLogger()},
			make(chan struct{}),
			time.Now(),
		)
		expectedCode := int(syscall.SIGTERM) + 128
		if errCode.Code != expectedCode {
			t.Errorf("Expected error code %d, got %d", expectedCode, errCode.Code)
		}
	})
}