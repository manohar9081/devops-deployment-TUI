// Item lists and selection parsing for the stack/cloud prompts: the Go
// port of K8S_STACK_ITEMS, CLOUD_ITEMS and tokens_from_selection from
// devops-deployment.sh's ask_stack_cloud().

package ask

import "strings"

// K8sStackItems are the choices for "which Kubernetes stack(s) do you
// use?", ported verbatim from K8S_STACK_ITEMS in devops-deployment.sh.
// The leading token of each line is the value stored in
// DEVOPS_K8S_STACKS (see TokensFromSelection); the rest is only there
// for the human reading the menu.
var K8sStackItems = []string{
	"rancher   — Rancher-managed clusters (kubeconfig from Rancher)",
	"openshift — Red Hat OpenShift (oc client)",
	"gke       — Google Kubernetes Engine (GCP)",
	"eks       — Amazon Elastic Kubernetes Service",
	"aks       — Azure Kubernetes Service",
	"plain     — plain kubeconfig / anything else",
}

// CloudItems are the choices for "which cloud provider(s) do you use?",
// ported verbatim from CLOUD_ITEMS in devops-deployment.sh; the leading
// token of each line becomes a DEVOPS_CLOUD_PROVIDERS value.
var CloudItems = []string{
	"aws    — Amazon Web Services",
	"gcp    — Google Cloud",
	"azure  — Microsoft Azure",
	"none   — on-prem / other / skip cloud tooling",
}

// TokensFromSelection ports tokens_from_selection: it takes the menu
// lines the user checked and keeps the first whitespace-separated token
// of each ("aws    — Amazon Web Services" → "aws"), skipping lines with
// no token, and space-joins the rest so the result reads like
// DEVOPS_K8S_STACKS="rancher gke". An empty selection yields "".
func TokensFromSelection(selected []string) string {
	tokens := make([]string, 0, len(selected))
	for _, line := range selected {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		tokens = append(tokens, fields[0])
	}
	return strings.Join(tokens, " ")
}
