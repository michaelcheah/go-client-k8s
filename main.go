package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	seldonclientset "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"seldon_test/deployer"
	"seldon_test/logger"
	"seldon_test/parse"
	"time"
)

func logWithTrace(err error) {
	if err != nil {

		log.Printf("%+v", errors.WithStack(err))
	}
}

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig_path", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig_path", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	logWithTrace(err)

	deployment, err := getSeldonDeployment("./seldon_deployment.json")

	ctx, _ := context.WithTimeout(context.Background(), time.Second * 160)

	customResourceDeployer, err := deployer.NewDeployer(config, deployment)
	logWithTrace(err)

	informer := logger.NewLogger(*customResourceDeployer.GetClientSet())

	err = customResourceDeployer.Create(ctx)
	logWithTrace(err)

	informer.First.RegisterFunc(func() error {
		fmt.Println("########## FIRST EVENT HAS BEEN TRIGGERED ")
		fmt.Println("########## Scaling DEPLOYMENT ")
		err := customResourceDeployer.ScaleReplicas(ctx, 2)
		fmt.Println("########## Scaled DEPLOYMENT ")
		return err
	})

	informer.Second.RegisterFunc(func() error {
		fmt.Println("########## SECOND EVENT HAS BEEN TRIGGERED ")
		fmt.Println("########## DELETING DEPLOYMENT ")
		err := customResourceDeployer.Delete(ctx)
		fmt.Println("########## DELETED DEPLOYMENT ")
		return err
	})

	informer.Third.RegisterFunc(func() error {
		fmt.Println("########## THIRD EVENT HAS BEEN TRIGGERED ")
		fmt.Println("########## Stopping informer")
		informer.Stop()
		fmt.Println("########## informer stoppedddd")
		return err
	})


	go informer.Run()

	informer.WaitTillEnd()


	//stop := make(chan struct{})
	//defer close(stop)
	//informerFactory.Start(stop)
	//start := time.Now()
	//deleted := false
	//for {
	//	time.Sleep(time.Second)
	//
	//	if time.Now().After(start.Add(time.Second *30)) && !deleted {
	//		customResourceDeployer.Finish(ctx)
	//		deleted = true
	//	}
	//	if time.Now().After(start.Add(time.Second *90)) {
	//		break
	//	}
	//}


	//err = customResourceDeployer.ScaleReplicas(ctx, 2)
	//logWithTrace(err)
}

func getSeldonDeployment(filepath string) (*machinelearningv1.SeldonDeployment, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open '%s'", filepath)
	}

	rawData, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get raw data from '%s'", filepath)
	}

	deployment, err := parse.UnmarshalSeldonDeployment(rawData)
	if err != nil {
		return nil, errors.Wrap(err, "could not unmarshal raw data into seldon deployment")
	}

	return deployment, nil
}

func printDeployment(deployment *machinelearningv1.SeldonDeployment) {
	if deployment == nil {
		log.Println("nil deployment")
		return
	}
	template := "%s\n\t%-32s%-8s\n"
	fmt.Printf(template, deployment.Name, deployment.Spec.Name, string(deployment.Status.State))
	fmt.Printf("%+v\n", deployment)
}

// printDeployments prints a list of SeldonDeployments on console
func printDeployments(deployments *machinelearningv1.SeldonDeploymentList) {
	if len(deployments.Items) == 0 {
		log.Println("No deployments found")
		return
	}
	template := "%-32s%-8s\n"
	fmt.Println("--- PVCs ----")
	fmt.Printf(template, "NAME", "STATUS")
	for _, deployment := range deployments.Items {
		fmt.Printf(template, deployment.Name, string(deployment.Status.State))
	}
}


func oldMain() {
	log.Println("me gusta")
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig_path", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig_path", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := seldonclientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}


	deployment, err := getSeldonDeployment("./seldon_deployment.json")
	if err != nil {
		panic(err)
	}
	log.Printf("%v\n", deployment)
	namespace := deployment.GetNamespace()
	if namespace == "" {
		namespace = v1.NamespaceDefault
	}
	deploymentClient := clientset.MachinelearningV1().SeldonDeployments(namespace)

	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	createOptions := metav1.CreateOptions{TypeMeta: metav1.TypeMeta{}}
	listOptions := metav1.ListOptions{
		TypeMeta:      metav1.TypeMeta{},
	}

	result, err := deploymentClient.Create(ctx, deployment, createOptions)
	if err != nil {
		log.Fatalf("%+v", errors.WithStack(err))
	}
	fmt.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())
	fmt.Println()

	deployments, err := deploymentClient.List(ctx, listOptions)
	if err != nil {
		log.Fatalf("%+v", errors.WithStack(err))
	}
	printDeployments(deployments)

	watcher, err := deploymentClient.Watch(ctx, listOptions)
	if err != nil {
		log.Fatalf("%+v", errors.WithStack(err))
	}
	ch := watcher.ResultChan()
	endTime := time.Now().Add(10 * time.Second)

	log.Println("watching...")
	for event := range ch {
		log.Printf("Got event of type: %v\n", reflect.TypeOf(event.Object))
		seldonDeployment, ok := event.Object.(*machinelearningv1.SeldonDeployment)
		if !ok {
			log.Printf("Got unexpected event of type: %v\n", reflect.TypeOf(event.Object))
		}
		printDeployment(seldonDeployment)
		if time.Now().After(endTime) {
			break
		}
	}


	delCtx, _ := context.WithTimeout(context.Background(), 5 * time.Second)
	delPolicy := metav1.DeletePropagationForeground
	delOptions := metav1.DeleteOptions{
		TypeMeta:           metav1.TypeMeta{},
		PropagationPolicy:  &delPolicy,
	}
	if err := deploymentClient.Delete(delCtx, deployment.GetObjectMeta().GetName(), delOptions); err != nil {
		log.Fatalf("%+v", errors.WithStack(err))
	}
	log.Println("deployment deleted")
}