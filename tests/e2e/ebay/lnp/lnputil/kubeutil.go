package lnputil

import (
	"fmt"
	"os"

	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func GetDynamicClient(kubeconfig string) dynamic.Interface {
	if len(kubeconfig) == 0 {
		kubeconfig = os.Getenv("KUBECONFIG")
		if len(kubeconfig) == 0 {
			panic(fmt.Errorf("no valid kubeconfig provided"))
		}
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return dynamicClient
}

func GetKubeClient(kubeconfig string) clientset.Interface {
	if len(kubeconfig) == 0 {
		kubeconfig = os.Getenv("KUBECONFIG")
		if len(kubeconfig) == 0 {
			panic(fmt.Errorf("no valid kubeconfig provided"))
		}
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	client, err := clientset.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return client
}
