package auth

import (
	"os"
	"testing"
)

func TestComputeCLIToken_Deterministic(t *testing.T) {
	t1 := ComputeCLIToken()
	t2 := ComputeCLIToken()
	if t1 == "" {
		t.Skip("no machine ID available in this environment")
	}
	if t1 != t2 {
		t.Errorf("expected same token across calls, got %q vs %q", t1, t2)
	}
}

func TestValidateCLIToken_Empty(t *testing.T) {
	if ValidateCLIToken("") {
		t.Error("expected false for empty token")
	}
}

func TestValidateCLIToken_Invalid(t *testing.T) {
	if ValidateCLIToken("random-string") {
		t.Error("expected false for random token")
	}
}

func TestValidateCLIToken_Valid(t *testing.T) {
	os.Setenv("MACHINE_ID", "test-machine-id")
	defer os.Unsetenv("MACHINE_ID")
	expected := ComputeCLIToken()
	if expected == "" {
		t.Skip("could not compute token")
	}
	if !ValidateCLIToken(expected) {
		t.Error("expected valid token to validate")
	}
}

func TestMachineID_EnvOverride(t *testing.T) {
	os.Setenv("MACHINE_ID", "env-machine-id")
	defer os.Unsetenv("MACHINE_ID")
	if got := machineID(); got != "env-machine-id" {
		t.Errorf("expected env-machine-id, got %q", got)
	}
}
