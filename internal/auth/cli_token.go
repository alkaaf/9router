package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
)

// CLITokenSalt is the canonical salt used for the CLI token derivation.
const CLITokenSalt = "9r-cli-auth"

// ComputeCLIToken returns a stable token for the current machine.
// Reads MACHINE_ID env var, falls back to hostname, then platform UUID.
func ComputeCLIToken() string {
	id := machineID()
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(CLITokenSalt + ":" + id))
	return hex.EncodeToString(sum[:])
}

// ValidateCLIToken compares the header value against the current
// machine token using constant-time equality.
func ValidateCLIToken(headerToken string) bool {
	if headerToken == "" {
		return false
	}
	expected := ComputeCLIToken()
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(headerToken)) == 1
}

func machineID() string {
	if v := os.Getenv("MACHINE_ID"); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return platformUUID()
}

func platformUUID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IOPlatformUUID") {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		}
	}
	return ""
}
