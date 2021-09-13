package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientset      kubernetes.Interface
	depLister      appslisters.DeploymentLister
	depCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
	interval       int
	namespaces     []string
}

func newController(clientset kubernetes.Interface, depInformer appsinformers.DeploymentInformer, interval int, namespaces []string) *controller {
	c := &controller{
		clientset:      clientset,
		depLister:      depInformer.Lister(),
		depCacheSynced: depInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), ""),
		interval:       interval,
		namespaces:     namespaces,
	}
	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.handleAdd,
		},
	)

	return c
}

func (c *controller) run(ch <-chan struct{}) {
	fmt.Println("starting controller")
	if !cache.WaitForCacheSync(ch, c.depCacheSynced) {
		fmt.Print("waiting for cache to be synced\n")
	}

	go wait.Until(c.worker, 1*time.Second, ch)

	<-ch
}

func (c *controller) worker() {
	for c.processItem() {

	}
}
func (c *controller) processItem() bool {
	item, shutdown := c.queue.Get()
	d := item.(*v1.Deployment)
	age := time.Since(d.CreationTimestamp.Time).Round(time.Minute)
	if shutdown {
		return false
	}
	if age > time.Duration(c.interval*60000*int(time.Millisecond)) {
		fmt.Printf("Restarting Deployment : %s\n", d.Name)
		err := c.clientset.AppsV1().Deployments(d.Namespace).Delete(context.Background(), d.Name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("Error while deleting the deployment : %s \n", d.Name)
		} else {
			c.restartDeployment(d.Namespace)
		}
		c.queue.Forget(item)
	} else {
		c.queue.Done(item)
		c.queue.Add(item)
	}
	return false
}

func (c *controller) handleAdd(obj interface{}) {
	d := obj.(*v1.Deployment)
	namespaceCheck := len(c.namespaces) == 0 || c.containsNameSpace(d.Namespace)
	mesh, _ := strconv.ParseBool(d.GetLabels()["mesh"])
	if namespaceCheck && mesh {
		fmt.Printf("Adding Deployment %s to the queue. \n", d.Name)
		c.queue.Add(obj)
	}
}

func (c *controller) restartDeployment(namespace string) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nginx-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "nginx",
					"mesh": "true",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":  "nginx",
						"mesh": "true",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.14.2",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
	labels := make(map[string]string)
	labels["app"] = "nginx"
	labels["mesh"] = "true"
	deployment.SetLabels(labels)
	_, err := c.clientset.AppsV1().Deployments(namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Error while creating a new deployment : %s \n", err.Error())
	}
}

func (c *controller) containsNameSpace(namespace string) bool {
	for _, ns := range c.namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func int32Ptr(i int32) *int32 { return &i }
