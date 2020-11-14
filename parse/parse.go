package parse

import (
	"github.com/akamensky/argparse"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

type ClientParser struct {
	parser *argparse.Parser
	args   ClientArgs // Pointer
}

type ClientArgs struct {
	Kubeconfig *string
	DeployConfig *string
	Debug      *bool
}

/*
Wrapper around argparse library to keep main application clean
*/
func NewClientParser() ClientParser {
	parser := argparse.NewParser("Go k8s client", "Deploys, scales, and deletes a k8s deployment")

	args := ClientArgs{}

	kubeConfigArgOptions := &argparse.Options{
		Help: "absolute path to kubeconfig file",
	}

	if home := homedir.HomeDir(); home != "" {
		kubeConfigArgOptions.Default = filepath.Join(home, ".kube", "config")
	} else {
		kubeConfigArgOptions.Required = true
	}

	args.Kubeconfig = parser.String("k", "kubeconfig", kubeConfigArgOptions)
	args.DeployConfig = parser.String("c", "config",  &argparse.Options{
		Default: "./seldon_deployment.json",
		Help:    "file path to deployment yaml/json file",
	})
	args.Debug = parser.Flag("d", "debug", &argparse.Options{
		Default: false,
		Help:    "debug flag. Warning: will be very spammy, only enable for debugging purposes",
	})

	return ClientParser{
		parser: parser,
		args:   args,
	}
}

func (c *ClientParser) Parse(args []string) (ClientArgs, error) {
	err := c.parser.Parse(args)
	if err != nil {
		return ClientArgs{}, err
	}
	return c.args, nil
}

// TODO: This is a hack. Look at k8s.io repo to see how yaml files are handled for structs with json tags
func convertToJsonBytes(rawData []byte) (rawJsonData []byte, err error) {
	var body interface{}
	if err = yaml.Unmarshal(rawData, &body); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal raw data")
	}
	if rawJsonData, err = json.Marshal(body); err != nil {
		return nil, errors.Wrap(err, "could not marshal data into json bytes")
	}
	return rawJsonData, nil
}

func UnmarshalSeldonDeployment(rawData []byte) (*machinelearningv1.SeldonDeployment, error) {
	rawJsonData, err := convertToJsonBytes(rawData)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert raw data to raw json data")
	}

	var deployment machinelearningv1.SeldonDeployment
	err = json.Unmarshal(rawJsonData, &deployment)

	if err != nil {
		return nil, errors.Wrap(err, "could not unmarshall raw data into SeldonDeployment type")
	}
	return &deployment, nil
}
