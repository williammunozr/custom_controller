package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"k8s.io/klog"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	var kubeconfig string
	var labelSelector string
	flag.StringVar(&kubeconfig, "kubeconfig", "/home/william/.kube/config", "absolute path to the kubeconfig file")
	flag.StringVar(&labelSelector, "labelselector", "zookeeper", "label to identify the pods")

	config, err := buildConfig(kubeconfig)
	if err != nil {
		klog.Fatal(err)
	}
	config.Timeout = 120 * time.Second

	// creates the clientset
	client := clientset.NewForConfigOrDie(config)

	// create the pod watcher
	/*optionsList := meta_v1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"status.phase": "Pending"}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"app": "zookeeper"}).String(),
	}
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), optionsList)
	if err != nil {
		klog.Fatalf("error getting pods: %v\n", err.Error())
	}

	for i := 0; i < len(pods.Items); i++ {
		fmt.Printf("Pod Name: %s - Pod Namespace: %s\n", pods.Items[i].ObjectMeta.Name, pods.Items[i].ObjectMeta.Namespace)
	}*/

	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.SelectorFromSet(fields.Set{"status.phase": "Pending"}).String()
		options.LabelSelector = labels.SelectorFromSet(labels.Set{"app": labelSelector}).String()
	}

	//optionsList := metav1.ListOptions{
	//	FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": "ip-172-101-2-232.us-west-2.compute.internal"}).String(),
	//}

	nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Fatal("error getting nodes: %v\n", err.Error())
	}

	for i := 0; i < len(nodes.Items); i++ {
		fmt.Printf("Node Name: %s\n", nodes.Items[i].ObjectMeta.Name)
	}

	podListWatcher := cache.NewFilteredListWatchFromClient(client.CoreV1().RESTClient(), "pods", metav1.NamespaceAll, optionsModifier)
	//podListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "pods", "", fields.SelectorFromSet(fields.Set{"metadata.name": podName, "status.phase": "Pending"}))

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the workqueue to a cache with the help of an informer.
	indexer, informer := cache.NewIndexerInformer(podListWatcher, &v1.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			fmt.Printf("add pod to queue\n")
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			fmt.Printf("delete pod from queue\n")
			if err == nil {
				queue.Forget(key)
			}
		},
	}, cache.Indexers{})

	controller := NewController(queue, indexer, informer)

	// Warm up the cache for initial synchronization.
	indexer.Add(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: labelSelector,
		},
	})

	// start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}
