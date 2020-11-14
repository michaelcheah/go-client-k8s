package deployer

import (
	"context"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

/*
DeploymentInstruction provides an interface for
 */
type DeploymentInstruction interface {
	Do(context.Context, *Deployer) error
	Done(event Event) (bool, error)
}

// TODO: These can be extended to be richer and contain more fields.
// TODO: More instructions can be added as needed. The don't even need to be deployment instructions
// e.g. Prompt? Or allow for model to be served?
type Create struct{}
type Delete struct{}
type ScaleReplicas struct {
	NumReplicas int32
}

func (c *Create) Do(ctx context.Context, d *Deployer) error {
	log.Info(ActionLog("Creating deployment..."))
	_, err := d.client.Create(ctx, d.deployment, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "could not create deployment")
	}
	return err
}

func (c *Create) Done(event Event) (bool, error) {
	if event.Deployment.Status.State == machinelearningv1.StatusStateAvailable {
		log.Info(MileStoneLog("Deployment is now available"))
		return true, nil
	}
	log.Info(EventLog("Deployment is not yet available"))
	return false, nil
}

func (s *ScaleReplicas) Do(ctx context.Context, d *Deployer) error {
	log.Infof(ActionLog("Scaling replicas to %d...", s.NumReplicas))
	// This should not exit until either successful or non-conflict error occurs
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, getErr := d.client.Get(ctx, d.name, metav1.GetOptions{})
		if getErr != nil {
			return errors.Wrapf(getErr, "could not get current deployment %s", d.name)
		}
		result.Spec.Replicas = int32Ptr(s.NumReplicas)
		_, updateErr := d.client.Update(ctx, result, metav1.UpdateOptions{})
		if updateErr != nil {
			log.Warnf("could not update deployment %s\n", d.name)
			// Return the error as is because it implements the APIStatus interface and will allow for retries on conflict
			// In particular, we expect the intermittent error: "Operation cannot be fulfilled on ... : the object has been modified; please apply your changes to the latest version and try again"
			// This is because between client.Get and client.Update, the SeldonDeployment could be modified.
			// Retrying ensures the latest SeldonDeployment is updated
			return updateErr
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to scale replicas")
	}
	return nil
}

func (s *ScaleReplicas) Done(event Event) (bool, error) {
	deploy := event.Deployment
	if deploy.Spec.Replicas != nil {
		if *deploy.Spec.Replicas == s.NumReplicas {
			log.Info(MileStoneLog("Replicas have been scaled to %d", s.NumReplicas))
			return true, nil
		}
	}
	return false, nil
}

func (d *Delete) Do(ctx context.Context, deploy *Deployer) error {
	log.Info(ActionLog("Deleting deployment..."))
	delPolicy := metav1.DeletePropagationBackground
	delOptions := metav1.DeleteOptions{
		PropagationPolicy: &delPolicy,
	}
	if err := deploy.client.Delete(ctx, deploy.name, delOptions); err != nil {
		return errors.Wrapf(err, "Failed to delete deployment")
	}
	return nil
}

func (d *Delete) Done(event Event) (bool, error) {
	if event.Type == Deleted {
		log.Info(MileStoneLog("Deployment has been deleted"))
		return true, nil
	}
	return false, nil
}
