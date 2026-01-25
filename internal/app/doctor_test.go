package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDoctorRepairsInvalidStateCurrent(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	repoInfo, err := resolveRepo(cfg, "")
	if err != nil {
		t.Fatalf("repo detection error: %v", err)
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		t.Fatalf("store open error: %v", err)
	}
	if err := st.EnsureRepo(repoInfo); err != nil {
		t.Fatalf("store repo error: %v", err)
	}
	if err := st.SetStateCurrent(repoInfo.ID, "default", "{bad json}", 0, time.Now().UTC()); err != nil {
		t.Fatalf("seed invalid state: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("store close error: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"doctor"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected doctor to fail on invalid state")
	}
	if !strings.Contains(errOut.String(), "invalid workspace state JSON (workspace=default)") {
		t.Fatalf("expected invalid state error, got: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "state_current: invalid (workspace=default)") {
		t.Fatalf("expected invalid state output, got: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = Run([]string{"doctor", "--repair"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected doctor repair to succeed, got: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "state_current: repaired -> {}") {
		t.Fatalf("expected repaired state output, got: %s", out.String())
	}

	st, err = openStore(cfg, repoInfo.ID)
	if err != nil {
		t.Fatalf("store open error: %v", err)
	}
	stateJSON, _, _, err := st.GetStateCurrent(repoInfo.ID, "default")
	if err != nil {
		t.Fatalf("state current error: %v", err)
	}
	if !json.Valid([]byte(stateJSON)) {
		t.Fatalf("expected valid state JSON, got: %s", stateJSON)
	}
	if strings.TrimSpace(stateJSON) != "{}" {
		t.Fatalf("expected repaired state to be {}, got: %s", stateJSON)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("store close error: %v", err)
	}

	out.Reset()
	errOut.Reset()
	code = Run([]string{"get", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected get to succeed after repair, got: %s", errOut.String())
	}
}
