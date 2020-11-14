package deployer

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	machinelearningv1 "github.com/seldonio/seldon-core/operator/apis/machinelearning.seldon.io/v1"
	seldonclientset "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned"
	seldondeployment "github.com/seldonio/seldon-core/operator/client/machinelearning.seldon.io/v1/clientset/versioned/typed/machinelearning.seldon.io/v1"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"time"
)

// TODO: Keep Deployer instance alive until events are finished
type Deployer struct {
	name       string
	eventChan  chan Event
	replyChan  chan error
	observer   *ObserverV2
	deployment *machinelearningv1.SeldonDeployment        // Schema/State of deployment
	client     seldondeployment.SeldonDeploymentInterface // Equivalent to kubernetes.DeploymentInterface
}

func NewDeployer(config *rest.Config, deployment *machinelearningv1.SeldonDeployment, debug bool) (deployer *Deployer, err error) {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	clientset, err := seldonclientset.NewForConfig(config)
	if err != nil {
		return deployer, errors.Wrapf(err, "could not create new Seldon ClientSet")
	}

	namespace := deployment.GetNamespace()
	if namespace == "" {
		namespace = v1.NamespaceDefault
		log.Warn(ThisNeedsAttentionLog("namespace was not provided. Using default namespace"))
	}
	if deployment.GetObjectMeta().GetName() == "" {
		return deployer, fmt.Errorf("deployment cannot have empty metadata.name")
	}

	client := clientset.MachinelearningV1().SeldonDeployments(namespace)

	// TODO: Have separate loggers for Deployer and Observer?
	log.Info("New deployment created...")

	deployer = &Deployer{
		name:       deployment.GetObjectMeta().GetName(),
		deployment: deployment,
		client:     client,
		eventChan:  make(chan Event),
		replyChan:  make(chan error),
	}

	deployer.observer = NewObserver(clientset)
	return deployer, nil
}

func (d *Deployer) RunInstructions(instructions []DeploymentInstruction) error {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 60*time.Second)
	// This should mean that the observers which hold this context will gracefully exit once all instrructions
	// have been executed
	defer cancelFunc()

	d.observer.NotifyFunc = func(event Event) error {
		return d.notifyFunc(ctx, event)
	}
	d.observer.ErrorFunc = func() {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		// Ignore error and just send delete call
		deleteFinalizer := Delete{}
		err := deleteFinalizer.Do(ctx, d)
		log.Errorf("got an error while cleaning up: %s", err)
	}
	go d.observer.Run()

	log.Info(EventLog("Start running instructions"))
	for _, instruction := range instructions {
		err := d.executeInstruction(ctx, instruction)
		if err != nil {
			return err
		}
	}
	log.Info(EventLog("Instructions have been run successfully"))
	return nil
}

func (d *Deployer) executeInstruction(ctx context.Context, instruction DeploymentInstruction) error {
	err := instruction.Do(ctx, d)
	if err != nil {
		return errors.Wrapf(err, "failed to carry out instruction")
	}
	err = d.waitForSpecificEvent(ctx, instruction.Done)
	if err != nil {
		return errors.Wrapf(err, "instruction error-ed before finishing")
	}
	return nil
}

func (d *Deployer) notifyFunc(ctx context.Context, event Event) error {
	if event.Deployment == nil {
		return fmt.Errorf("received an event with nil Deployment")
	}
	select {
	case d.eventChan <- event:
	case <-ctx.Done():
		log.Errorf("received an event with nil Deployment")
		return fmt.Errorf("deployment context cancelled")
	}
	return nil
}

func (d *Deployer) waitForSpecificEvent(ctx context.Context, condition func(Event) (bool, error)) error {
	for {
		select {
		case event := <-d.eventChan:
			conditionSatisfied, err := condition(event)
			if err != nil {
				return err
			} else if conditionSatisfied {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while trying to satisfy event condition")
		}
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}
