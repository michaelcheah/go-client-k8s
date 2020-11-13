package parse

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	"github.com/stretchr/testify/assert"
)

func checkErrWithStackTrace(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("%+v", errors.WithStack(err))
	}
}

func TestUnmarshalSeldonDeployment(t *testing.T) {
	assertValues := func(deployment *machinelearningv1.SeldonDeployment) {
		assert.Equal(t, "SeldonDeployment", deployment.GetObjectKind().(*v1.TypeMeta).Kind)
		assert.Equal(t, "seldon-deployment-example", deployment.ObjectMeta.GetObjectMeta().GetName())

		assert.NotNil(t, deployment.Spec)
		assert.Equal(t, deployment.Spec.Name, "sklearn-iris-deployment")
		assert.Len(t, deployment.Spec.Predictors, 1)

		predictor := deployment.Spec.Predictors[0]
		assert.Len(t, predictor.ComponentSpecs, 1)
		assert.Len(t, predictor.ComponentSpecs[0].Spec.Containers, 1)

		container := predictor.ComponentSpecs[0].Spec.Containers[0]
		assert.Equal(t, "seldonio/sklearn-iris:0.12", container.Image)
		assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)
		assert.Equal(t, "sklearn-iris-classifier", container.Name)

		graph := predictor.Graph
		assert.NotNil(t, graph.Children)
		assert.NotNil(t, graph.Endpoint)
		assert.Equal(t, machinelearningv1.REST, graph.Endpoint.Type)
		assert.Equal(t, machinelearningv1.MODEL, *graph.Type)
		assert.Equal(t, int32(1), *predictor.Replicas)
	}

	t.Run("yaml", func(t *testing.T) {
		rawYamlData := []byte(`apiVersion: machinelearning.seldon.io/v1alpha2
kind: SeldonDeployment
metadata:
  name: seldon-deployment-example
spec:
  name: sklearn-iris-deployment
  predictors:
    - componentSpecs:
        - spec:
            containers:
              - image: seldonio/sklearn-iris:0.12
                imagePullPolicy: IfNotPresent
                name: sklearn-iris-classifier
      graph:
        children: []
        endpoint:
          type: REST
        type: MODEL
      replicas: 1`)

		deployment, err := UnmarshalSeldonDeployment(rawYamlData)
		checkErrWithStackTrace(t, err)

		assertValues(deployment)
	})

	t.Run("json", func(t *testing.T) {
		rawJsonData := []byte(`{
  "apiVersion": "machinelearning.seldon.io/v1alpha2",
  "kind": "SeldonDeployment",
  "metadata": {
    "name": "seldon-deployment-example"
  },
  "spec": {
    "name": "sklearn-iris-deployment",
    "predictors": [
      {
        "componentSpecs": [
          {
            "spec": {
              "containers": [
                {
                  "image": "seldonio/sklearn-iris:0.12",
                  "imagePullPolicy": "IfNotPresent",
                  "name": "sklearn-iris-classifier"
                }
              ]
            }
          }
        ],
        "graph": {
          "children": [],
          "endpoint": {
            "type": "REST"
          },
          "type": "MODEL"
        },
        "replicas": 1
      }
    ]
  }
}`)
		deployment, err := UnmarshalSeldonDeployment(rawJsonData)
		if err != nil {
			checkErrWithStackTrace(t, err)
		}
		assertValues(deployment)
	})
}
