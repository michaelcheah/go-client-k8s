package parse

import (
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/json"
)

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

