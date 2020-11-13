package deployer

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	seldonclientset "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned"
	seldondeployment "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned/typed/machinelearning.seldon.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"log"

	//"k8s.io/apimachinery/pkg/fields"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

// TODO: Keep Deployer instance alive until events are finished
type Deployer struct {
	namespace        string
	name string
	clientset *seldonclientset.Clientset
	deployment       *machinelearningv1.SeldonDeployment        // schema
	client           seldondeployment.SeldonDeploymentInterface // Equivalent to kubernetes.DeploymentInterface
}

func NewDeployer(config *rest.Config, deployment *machinelearningv1.SeldonDeployment) (deployer Deployer, err error) {
	clientset, err := seldonclientset.NewForConfig(config)
	if err != nil {
		return deployer, errors.Wrapf(err, "could not create new Seldon ClientSet")
	}

	namespace := deployment.GetNamespace()
	if namespace == "" {
		namespace = v1.NamespaceDefault
	}
	if deployment.GetObjectMeta().GetName() == "" {
		return deployer, fmt.Errorf("deployment cannot have empty metadata.name")
	}

	client := clientset.MachinelearningV1().SeldonDeployments(namespace)

	fmt.Println("new deployment created...")

	return Deployer{
		namespace:  namespace,
		name: deployment.GetObjectMeta().GetName(),
		clientset: clientset,
		deployment: deployment,
		client:     client,
	}, nil
}

func (d *Deployer) GetClientSet() *seldonclientset.Clientset {
	return d.clientset
}

func (d *Deployer) Create(ctx context.Context) error {
	fmt.Println("creating deployment...")
	_, err := d.client.Create(ctx, d.deployment, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not create deployment")
	}
	return nil
}

func (d *Deployer) ScaleReplicas(ctx context.Context, numReplicas int32) error {
	fmt.Println("scaling replicas...")
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, getErr := d.client.Get(ctx, d.name, metav1.GetOptions{})
		if getErr != nil {
			return errors.Wrapf(getErr, "could not get current deployment %s", d.name)
		}
		result.Spec.Replicas = int32Ptr(numReplicas)
		_, updateErr := d.client.Update(ctx, result, metav1.UpdateOptions{})
		if updateErr != nil {
			log.Printf("could not update deployment %s\n", d.name)
			// Return the error as it implements the APIStatus interface and will allow for retries on conflict
			// In particular, we expect the intermittent error: "Operation cannot be fulfilled on ... : the object has been modified; please apply your changes to the latest version and try again"
			// This is because between client.Get and client.Update, the SeldonDeployment could be modified.
			// Retrying ensures the latest SeldonDeployment is updated
			return updateErr
		}
		return nil
	})
}

func (d *Deployer) Delete(ctx context.Context) error {
	fmt.Println("deleting deployment...")
	delPolicy := metav1.DeletePropagationBackground
	delOptions := metav1.DeleteOptions{
		PropagationPolicy:  &delPolicy,
	}
	if err := d.client.Delete(ctx, d.name, delOptions); err != nil {
		return errors.Wrapf(err, "failed to delete deployment")
	}
	return nil
}

func (d *Deployer) Finish(ctx context.Context) {
	err := d.Delete(ctx)
	if err != nil {
		fmt.Printf("%+v\n", errors.WithStack(err))
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}
