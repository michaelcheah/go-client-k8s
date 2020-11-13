package main

import (
	"context"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	"io/ioutil"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
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

	parser := parse.NewClientParser()
	args, err := parser.Parse(os.Args)
	logWithTrace(err)

	config, err := clientcmd.BuildConfigFromFlags("", *args.Kubeconfig)
	logWithTrace(err)

	deployment, err := getSeldonDeployment("./seldon_deployment.json")

	ctx, _ := context.WithTimeout(context.Background(), time.Second * 160)

	customResourceDeployer, err := deployer.NewDeployer(config, deployment)
	logWithTrace(err)

	informer := logger.NewLogger(*customResourceDeployer.GetClientSet(), *args.Debug)

	err = customResourceDeployer.Create(ctx)
	logWithTrace(err)

	informer.First.RegisterFunc(func() error {
		err := customResourceDeployer.ScaleReplicas(ctx, 2)
		return err
	})

	informer.Second.RegisterFunc(func() error {
		err := customResourceDeployer.Delete(ctx)
		return err
	})

	informer.Third.RegisterFunc(func() error {
		informer.Stop()
		return err
	})

	go informer.Run()

	informer.WaitTillEnd()
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
