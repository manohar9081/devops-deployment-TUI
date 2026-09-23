package ask

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"devops-deployment-go/internal/term"
)

func TestTokensFromSelection(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"nil selection", nil, ""},
		{"empty slice", []string{}, ""},
		{
			"single cloud item keeps its first token",
			[]string{"aws    — Amazon Web Services"},
			"aws",
		},
		{
			"several lines space-joined in order",
			[]string{
				"gke       — Google Kubernetes Engine (GCP)",
				"eks       — Amazon Elastic Kubernetes Service",
				"rancher   — Rancher-managed clusters (kubeconfig from Rancher)",
			},
			"gke eks rancher",
		},
		{
			"whole K8S_STACK_ITEMS list",
			K8sStackItems,
			"rancher openshift gke eks aks plain",
		},
		{
			"blank lines are skipped",
			[]string{"", "   ", "azure  — Microsoft Azure"},
			"azure",
		},
		{
			"leading whitespace does not become part of the token",
			[]string{"\t  none   — on-prem / other / skip cloud tooling"},
			"none",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TokensFromSelection(tc.in); got != tc.want {
				t.Errorf("TokensFromSelection(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMultiSelectRender(t *testing.T) {
	t.Setenv("NO_COLOR", "1") // pin the plain path: no ANSI either way

	ms := &MultiSelect{
		Title:  "Which Kubernetes stack(s) do you use? (Space to check, Enter to accept)",
		Prompt: "k8s> ",
		Items:  K8sStackItems[:3],
	}
	want := strings.Join([]string{
		"\r\x1b[2K" + ms.Title,
		"\r\x1b[2K",
		"\r\x1b[2K" + ms.Prompt,
		"\r\x1b[2K  [ ] " + ms.Items[0],
		"\r\x1b[2K> [x] " + ms.Items[1],
		"\r\x1b[2K  [x] " + ms.Items[2],
		"",
	}, "\n")

	var buf bytes.Buffer
	lines := ms.render(&buf, 1, map[int]bool{1: true, 2: true})

	if got := buf.String(); got != want {
		t.Errorf("render output:\n%q\nwant:\n%q", got, want)
	}
	if lines != 6 {
		t.Errorf("render lines = %d, want 6 (title + blank + prompt + 3 items)", lines)
	}
	if n := strings.Count(buf.String(), "\n"); n != 6 {
		t.Errorf("render drew %d newlines, want 6, so the cursor is not one line below the block", n)
	}
	// \x1b[2K is cursor control, not color, so it is written even under
	// NO_COLOR (menu Draw does the same); nothing beyond it may appear.
	if plain := strings.ReplaceAll(buf.String(), "\x1b[2K", ""); strings.Contains(plain, "\x1b") {
		t.Errorf("NO_COLOR is set but render emitted ANSI escapes beyond \\x1b[2K: %q", buf.String())
	}
}

func TestMultiSelectRenderNoANSIOnNonTTY(t *testing.T) {
	if term.IsTTY(os.Stdout.Fd()) {
		t.Skip("stdout is a terminal; the color path would be active")
	}
	t.Setenv("NO_COLOR", "") // unset: only the not-a-tty check keeps colors off

	ms := &MultiSelect{Title: "T", Prompt: "p> ", Items: []string{"aws    — Amazon Web Services"}}
	var buf bytes.Buffer
	ms.render(&buf, 0, map[int]bool{0: true})

	// \x1b[2K is cursor control, not color, so it is written even on a
	// non-TTY stdout (menu Draw does the same); nothing beyond it may
	// appear.
	if plain := strings.ReplaceAll(buf.String(), "\x1b[2K", ""); strings.Contains(plain, "\x1b") {
		t.Errorf("stdout is not a TTY but render emitted ANSI escapes beyond \\x1b[2K: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "> [x] "+ms.Items[0]) {
		t.Errorf("cursor row missing marker/checkbox, got %q", buf.String())
	}
}
