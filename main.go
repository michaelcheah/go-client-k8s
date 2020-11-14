package main

import (
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	"io/ioutil"
	"k8s.io/client-go/tools/clientcmd"
	log "github.com/sirupsen/logrus"
	"os"
	"seldon_test/deployer"
	"seldon_test/parse"
)

func logWithTrace(err error) {
	if err != nil {
		log.Errorf("%+v", errors.WithStack(err))
	}
}

func main() {

	parser := parse.NewClientParser()
	args, err := parser.Parse(os.Args)
	logWithTrace(err)

	config, err := clientcmd.BuildConfigFromFlags("", *args.Kubeconfig)
	logWithTrace(err)

	deployment, err := getSeldonDeployment(*args.DeployConfig)
	logWithTrace(err)

	customResourceDeployer, err := deployer.NewDeployer(config, deployment, *args.Debug)
	logWithTrace(err)

	customResourceDeployer.RunInstructions([]deployer.DeploymentInstruction{
		&deployer.Create{},
		&deployer.ScaleReplicas{NumReplicas: 2},
		&deployer.Delete{},
	})
	logWithTrace(err)
	log.Warn(deployer.MileStoneLog("HELLO"))
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
