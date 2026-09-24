package shell

import (
	"fmt"
	"os"
	"path/filepath"

	"archuser.org/muse/internal/composer"
)

// SetupInstructions only describes an opt-in edit. It never writes startup files
// or launches a shell. An absolute executable path also works from a checkout.
func SetupInstructions(shellPath, executable string) string {
	name := filepath.Base(shellPath)
	if !Supported(shellPath) {
		return "Muse supports automatic integration with Bash and Zsh.\nRun muse --shell bash or muse --shell zsh for the startup-file line.\nFor other shells, use muse composer --manual or muse generate --print.\n"
	}
	rc := "~/.bashrc"
	if name == "zsh" {
		rc = "~/.zshrc"
		if dir := os.Getenv("ZDOTDIR"); dir != "" {
			rc = filepath.Join(dir, ".zshrc")
		}
	}
	line := fmt.Sprintf("eval \"$(%s integration %s)\"", composer.Quote(executable), name)
	text := fmt.Sprintf("Activate Muse in your existing %s shell.\n\nManually add this line to %s:\n\n  %s\n\nOpen a new terminal afterward; Muse integration will activate automatically.\nPress Ctrl+X, then g at an empty prompt to open the composer.\n", name, composer.Display(rc), composer.Display(line))
	if name == "zsh" {
		text += "You can also run muse composer or muse generate REQUEST to stage text in your prompt.\n"
	}
	return text + "Accepted text remains editable. Only your separate Enter executes it.\nMuse has not modified your startup files.\n"
}
