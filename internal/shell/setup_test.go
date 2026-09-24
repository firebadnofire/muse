package shell

import (
	"strings"
	"testing"
)

func TestSetupInstructions(t *testing.T) {
	t.Setenv("ZDOTDIR", "/tmp/custom zsh")
	for _, tc := range []struct{ shell, rc string }{{"/bin/bash", "~/.bashrc"}, {"/usr/bin/zsh", "/tmp/custom zsh/.zshrc"}} {
		got := SetupInstructions(tc.shell, "/tmp/muse checkout/bin/muse")
		if !strings.Contains(got, tc.rc) || !strings.Contains(got, `eval "$('`+"/tmp/muse checkout/bin/muse' integration ") || !strings.Contains(got, "Open a new terminal") {
			t.Fatal(got)
		}
	}
	if got := SetupInstructions("/bin/fish", "muse"); strings.Contains(got, "eval") {
		t.Fatal(got)
	}
}
