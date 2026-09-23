// AskStackCloud: the Go port of ask_stack_cloud() from
// devops-deployment.sh, which asks which Kubernetes stack(s) and cloud
// provider(s) the user uses and saves the answers to <root>/config.env.

package ask

import (
	"errors"
	"fmt"
	"path/filepath"

	"devops-deployment-go/internal/config"
	"devops-deployment-go/internal/term"
)

// AskStackCloud runs the two multi-choice questions that fill
// DEVOPS_K8S_STACKS / DEVOPS_CLOUD_PROVIDERS and saves them to
// <root>/config.env, like ask_stack_cloud() in devops-deployment.sh. It
// reports whether the questions were actually asked; the caller keeps
// going either way.
//
// rd is the program's shared keystroke Reader (term.NewReaderStdin), used
// for both pickers so an escape window left open here parks its follow-up
// byte on the Reader the caller reads next, not on an abandoned one. It
// may be nil when the caller has no keystroke Reader (the line-based
// fallback menu): the terminal check below runs before rd is touched, and
// only once stdin is known to be a terminal does a nil rd get a fresh
// Reader — sequentially safe, because such a caller reads the stream no
// further after this call.
//
// Cancelling a question (Escape/q, or stdin ending) keeps the existing
// config: it prints `Cancelled — config unchanged.` and returns without
// saving. Answering, however, always proceeds — the port's one deliberate
// difference from bash, where an accepted-but-empty multi-select cancels
// the whole flow (`if [[ -z "$sel" ]]` treats it exactly like a cancel).
// Here Enter accepts the defaults in that case (stacks "rancher", clouds
// "none"), and the config is saved only once both questions have been
// answered — a cancel on either question still changes nothing.
func AskStackCloud(root string, rd *term.Reader) (asked bool, err error) {
	// No interactive terminal: do not guess and do not overwrite existing
	// config. bash probes for /dev/tty; the port checks stdin, which is
	// what MultiSelect reads. This check deliberately precedes any use of
	// rd, so a nil reader is only ever materialized once stdin is known
	// to be a terminal.
	if !term.IsTTY(0) {
		fmt.Println("Stack/cloud selection needs a terminal — keeping existing config.")
		return false, nil
	}
	if rd == nil {
		rd = term.NewReaderStdin()
	}

	sel, err := (&MultiSelect{
		Title:  "Which Kubernetes stack(s) do you use? (Space to check, Enter to accept)",
		Prompt: "k8s> ",
		Items:  K8sStackItems,
	}).Run(rd)
	if err != nil {
		if errors.Is(err, ErrCancelled) {
			fmt.Println("Cancelled — config unchanged.")
			return false, nil
		}
		return false, err
	}
	// Enter always proceeds (see above): an empty accept takes the bash
	// fallback default, `[[ -z "$stacks" ]] && stacks="rancher"`.
	stacks := TokensFromSelection(sel)
	if stacks == "" {
		stacks = "rancher"
	}

	sel, err = (&MultiSelect{
		Title:  "Which cloud provider(s) do you use? (Space to check, Enter to accept)",
		Prompt: "cloud> ",
		Items:  CloudItems,
	}).Run(rd)
	if err != nil {
		if errors.Is(err, ErrCancelled) {
			fmt.Println("Cancelled — config unchanged.")
			return false, nil
		}
		return false, err
	}
	// Same Enter-always-proceeds treatment for the clouds question: an
	// empty accept takes `[[ -z "$clouds" ]] && clouds="none"`.
	clouds := TokensFromSelection(sel)
	if clouds == "" {
		clouds = "none"
	}

	if err := config.Save(root, stacks, clouds); err != nil {
		return true, err
	}
	fmt.Printf("Saved: k8s stack(s) = %s · cloud = %s  (%s)\n",
		stacks, clouds, filepath.Join(root, "config.env"))
	PrintStackNotes()
	return true, nil
}
