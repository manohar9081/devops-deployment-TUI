// The stack/cloud notes: the Go port of print_stack_notes() from
// devops-deployment.sh, shown after the prerequisite checks and again
// after ask_stack_cloud() saves a fresh config.env.

package ask

import (
	"fmt"
	"os"
	"strings"

	"devops-deployment-go/internal/config"
)

// k8sStackNotes mirrors the rancher/openshift/gke/eks/aks/plain cases
// of print_stack_notes() in devops-deployment.sh.
var k8sStackNotes = map[string]string{
	"rancher":   "  · rancher   : k8screds merges ~/k8sconfig/*.yaml (Rancher kubeconfigs) into ~/.kube/config",
	"openshift": "  · openshift : install the oc client from the menu, then 'oc login --token=… --server=…'",
	"gke":       "  · gke       : needs gke-gcloud-auth-plugin — gcloud components install gke-gcloud-auth-plugin",
	"eks":       "  · eks       : login with 'aws eks update-kubeconfig --name <cluster> --region <region>'",
	"aks":       "  · aks       : login with 'az aks get-credentials --resource-group <rg> --name <cluster>'",
	"plain":     "  · plain     : drop kubeconfig YAMLs into ~/k8sconfig/ and run k8screds",
}

// cloudProviderNotes mirrors the aws/gcp/azure cases of
// print_stack_notes(); "none" prints nothing, like the bash original,
// so it is simply absent here.
var cloudProviderNotes = map[string]string{
	"aws":   "  · aws   : AWS CLI + SSM plugin via the menu (option 5)",
	"gcp":   "  · gcp   : install Google Cloud SDK (gcloud) separately; brew install google-cloud-sdk on macOS",
	"azure": "  · azure : install az CLI separately; brew install azure-cli on macOS",
}

// PrintStackNotes ports print_stack_notes() from devops-deployment.sh:
// it reminds the user how to authenticate for each selected Kubernetes
// stack and cloud provider, reading them from the DEVOPS_K8S_STACKS /
// DEVOPS_CLOUD_PROVIDERS environment variables. Like the bash original
// it is a no-op unless DEVOPS_K8S_STACKS is set.
func PrintStackNotes() {
	stacks := os.Getenv(config.KeyK8sStacks)
	if stacks == "" {
		return
	}

	fmt.Println()
	fmt.Printf("Selected Kubernetes stack(s): %s\n", stacks)
	for _, s := range strings.Fields(stacks) {
		if note, ok := k8sStackNotes[s]; ok {
			fmt.Println(note)
		}
	}

	if clouds := os.Getenv(config.KeyCloudProviders); clouds != "" {
		fmt.Printf("Selected cloud provider(s): %s\n", clouds)
		for _, s := range strings.Fields(clouds) {
			if note, ok := cloudProviderNotes[s]; ok {
				fmt.Println(note)
			}
		}
	}
}
